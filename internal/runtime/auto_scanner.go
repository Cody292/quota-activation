package runtime

import (
	"context"
	"time"

	"quota-activation/internal/activator"
	"quota-activation/internal/config"
	"quota-activation/internal/detector"
	"quota-activation/internal/host"
	"quota-activation/internal/state"
)

type autoScanSnapshot struct {
	host      host.Client
	activator *activator.Activator
	store     *state.Store
	config    config.Config
	now       func() time.Time
}

type autoCandidate struct {
	authID   string
	provider detector.Provider
	model    string
	disabled bool
	payload  []byte
}

func (r *Runtime) runAutoScanLoop(ctx context.Context) {
	r.scanAutoCandidates(ctx)
	ticker := time.NewTicker(r.autoScannerInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.scanAutoCandidates(ctx)
		}
	}
}

func (r *Runtime) autoScannerInterval() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.autoScanInterval <= 0 {
		return config.Default().ScanInterval
	}
	return r.autoScanInterval
}

func (r *Runtime) scanAutoCandidates(ctx context.Context) {
	snapshot, ok := r.autoSnapshot()
	if !ok {
		return
	}
	files, err := snapshot.host.ListAuthFiles(ctx)
	if err != nil {
		return
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return
		}
		runtimeAuth := snapshot.runtimeAuthFile(ctx, file)
		if err := ctx.Err(); err != nil {
			return
		}
		candidate, ok := snapshot.autoCandidate(file, runtimeAuth)
		if !ok {
			continue
		}
		_ = snapshot.activateCandidate(ctx, candidate)
	}
}

func (r *Runtime) autoSnapshot() (autoScanSnapshot, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shutdown || !r.config.AutoActivate || r.host == nil || r.activator == nil || r.store == nil {
		return autoScanSnapshot{}, false
	}
	return autoScanSnapshot{host: r.host, activator: r.activator, store: r.store, config: r.config, now: r.now}, true
}

func (s autoScanSnapshot) runtimeAuthFile(ctx context.Context, file host.AuthFile) autoRuntimeAuthFile {
	authIndex := firstNonBlankAuto(file.AuthIndex)
	if authIndex == "" {
		return autoRuntimeAuthFile{}
	}
	runtimeFile, err := s.host.GetRuntimeAuthFile(ctx, authIndex)
	if err != nil {
		return autoRuntimeAuthFile{}
	}
	return autoRuntimeAuthFile{file: runtimeFile, ok: true}
}

func (s autoScanSnapshot) activateCandidate(ctx context.Context, candidate autoCandidate) error {
	observedAt := s.now().UTC()
	decision, err := detector.Evaluate(detector.ProbeInput{AuthID: candidate.authID, Provider: candidate.provider, Model: candidate.model, ObservedAt: observedAt, Payload: candidate.payload}, "")
	if err != nil || !decision.Activate {
		return err
	}
	key := state.RecordKey{AuthID: candidate.authID, Provider: string(candidate.provider), Window: string(decision.Observation.Window), CycleKey: decision.CycleKey.String()}
	if record, exists := s.store.Record(key); exists && record.LastResult == string(activator.StatusSuccess) {
		return nil
	}
	_, err = s.activator.Activate(ctx, activator.Request{
		AuthID:     candidate.authID,
		Provider:   candidate.provider,
		ModelGroup: decision.Observation.ModelGroup,
		Window:     decision.Observation.Window,
		CycleKey:   decision.CycleKey,
		Model:      candidate.model,
		Disabled:   candidate.disabled,
		ObservedAt: observedAt,
		ResetAt:    decision.Observation.ResetAt,
	})
	return err
}
