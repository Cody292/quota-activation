package activator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"quota-activation/internal/config"
	"quota-activation/internal/host"
	"quota-activation/internal/session"
	"quota-activation/internal/state"
)

// Activator 串行执行一次性 quota 激活请求。
type Activator struct {
	host     host.Client
	sessions *session.Manager
	store    *state.Store
	config   config.Config
	now      func() time.Time
	runMu    sync.Mutex
}

// New 构造纯内部激活执行器。
func New(options Options) *Activator {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Activator{host: options.Host, sessions: options.Sessions, store: options.State, config: options.Config, now: now}
}

// Activate 创建一次性 session nonce，调用 host.model.execute，并写入脱敏状态。
func (a *Activator) Activate(ctx context.Context, request Request) (Result, error) {
	result := a.initialResult(request)
	if !a.runMu.TryLock() {
		result.Status = StatusBusy
		return result, ErrBusy
	}
	defer a.runMu.Unlock()

	if err := a.checkDependencies(); err != nil {
		return a.failAndStore(ctx, result, err)
	}
	normalized, err := a.normalizeRequest(request)
	result = a.initialResult(normalized)
	if err != nil {
		return a.failAndStore(ctx, result, err)
	}
	if normalized.Disabled && !a.config.EnableBeforeActivation {
		return a.skipDisabled(ctx, result)
	}

	restore, enableErr := a.enableIfNeeded(ctx, normalized)
	if enableErr != nil {
		return a.failAndStore(ctx, result, enableErr)
	}
	if restore != nil {
		result.TemporaryEnabled = true
	}

	result, activateErr := a.execute(ctx, normalized, result)
	if restore != nil {
		restoreErr := restore(ctx)
		result.RestoredDisabled = restoreErr == nil
		if restoreErr != nil {
			activateErr = errors.Join(activateErr, fmt.Errorf("restore disabled credential: %w", safeError(restoreErr)))
		}
	}
	if activateErr != nil {
		return a.failAndStore(ctx, result, activateErr)
	}
	result.Status = StatusSuccess
	result.Success = true
	return a.storeResult(ctx, result)
}

func (a *Activator) checkDependencies() error {
	if a == nil || a.host == nil || a.sessions == nil || a.store == nil {
		return ErrMissingDependency
	}
	return nil
}

func (a *Activator) skipDisabled(ctx context.Context, result Result) (Result, error) {
	result.Status = StatusSkipped
	result.LastError = state.Redact(ErrDisabledCredential.Error())
	stored, storeErr := a.storeResult(ctx, result)
	if storeErr != nil {
		return stored, errors.Join(ErrDisabledCredential, storeErr)
	}
	return stored, ErrDisabledCredential
}
