package runtime

import (
	"context"
	"strings"
	"time"

	"quota-activation/internal/activator"
	"quota-activation/internal/config"
	"quota-activation/internal/detector"
	"quota-activation/internal/host"
	"quota-activation/internal/state"
)

const maxSchedulerMissCooldown = 5 * time.Minute
const schedulerMissCooldownReason = "调度未选中目标凭证（已冷却）"
const schedulerMissErrorMessage = "调度器未选中目标凭证"
const hostExecutorUnavailableCooldownReason = "宿主模型执行器不可用（已冷却）"
const hostExecutorUnavailableMessage = "宿主模型执行器不可用（宿主未就绪或回调失败，非凭证失效）"

type autoScanSnapshot struct {
	host                       host.Client
	activator                  *activator.Activator
	store                      *state.Store
	config                     config.Config
	now                        func() time.Time
	shouldSkipActivateCooldown func(authID string) bool
	noteActivateCooldown       func(authID, reason string)
	cooldownSkipReason         func(authID string) string
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
	if !r.claimAutoScanSlot() {
		return
	}
	files, err := snapshot.host.ListAuthFiles(ctx)
	if err != nil {
		r.snapshotScanSummary(scanSummaryInput{
			Attempted: 0, Succeeded: 0, Failed: 1, Skipped: 0,
			FailReasons: map[string]int{"列举凭证失败": 1},
		})
		return
	}
	acc := newScanSummaryAccumulator()
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			r.snapshotScanSummary(acc.toInput())
			return
		}
		runtimeAuth := snapshot.runtimeAuthFile(ctx, file)
		if err := ctx.Err(); err != nil {
			r.snapshotScanSummary(acc.toInput())
			return
		}
		candidate, ok := snapshot.autoCandidate(file, runtimeAuth)
		if !ok {
			continue
		}
		detail := snapshot.activateCandidateDetail(ctx, candidate)
		acc.add(string(candidate.provider), detail)
	}
	// 每个 auto tick 只写一条合并摘要，禁止同 tick 再写 activation。
	r.snapshotScanSummary(acc.toInput())
}

func (r *Runtime) autoSnapshot() (autoScanSnapshot, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shutdown || !r.config.AutoActivate || r.host == nil || r.activator == nil || r.store == nil {
		return autoScanSnapshot{}, false
	}
	return autoScanSnapshot{
		host: r.host, activator: r.activator, store: r.store, config: r.config, now: r.now,
		shouldSkipActivateCooldown: r.activateCooldownActive,
		noteActivateCooldown:       r.markActivateCooldownWithReason,
		cooldownSkipReason:         r.activateCooldownReason,
	}, true
}

func (r *Runtime) claimAutoScanSlot() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	interval := r.autoScanInterval
	if interval <= 0 {
		interval = r.config.ScanInterval
	}
	if interval <= 0 {
		interval = config.Default().ScanInterval
	}
	now := r.now()
	if !r.lastAutoScanAt.IsZero() && now.Sub(r.lastAutoScanAt) < interval {
		return false
	}
	r.lastAutoScanAt = now
	return true
}

func (r *Runtime) schedulerMissCooldownDuration() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.schedulerMissCooldownDurationLocked()
}

func (r *Runtime) schedulerMissCooldownDurationLocked() time.Duration {
	interval := r.autoScanInterval
	if interval <= 0 {
		interval = r.config.ScanInterval
	}
	if interval <= 0 {
		interval = config.Default().ScanInterval
	}
	if interval > maxSchedulerMissCooldown {
		return maxSchedulerMissCooldown
	}
	if interval <= 0 {
		return maxSchedulerMissCooldown
	}
	return interval
}

func (r *Runtime) activateCooldownActive(authID string) bool {
	if authID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	until, ok := r.schedulerMissUntil[authID]
	if !ok {
		return false
	}
	now := r.now()
	if now.Before(until) {
		return true
	}
	delete(r.schedulerMissUntil, authID)
	delete(r.activateCooldownReasonByAuth, authID)
	return false
}

func (r *Runtime) activateCooldownReason(authID string) string {
	if authID == "" {
		return schedulerMissCooldownReason
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if reason, ok := r.activateCooldownReasonByAuth[authID]; ok && reason != "" {
		return reason
	}
	return schedulerMissCooldownReason
}

func (r *Runtime) markActivateCooldownWithReason(authID, reason string) {
	if authID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.schedulerMissUntil == nil {
		r.schedulerMissUntil = make(map[string]time.Time)
	}
	if r.activateCooldownReasonByAuth == nil {
		r.activateCooldownReasonByAuth = make(map[string]string)
	}
	cooldown := r.schedulerMissCooldownDurationLocked()
	r.schedulerMissUntil[authID] = r.now().Add(cooldown)
	if reason == "" {
		reason = schedulerMissCooldownReason
	}
	r.activateCooldownReasonByAuth[authID] = reason
}

func (r *Runtime) schedulerMissCooldownActive(authID string) bool {
	return r.activateCooldownActive(authID)
}

func (r *Runtime) markSchedulerMiss(authID string) {
	r.markActivateCooldownWithReason(authID, schedulerMissCooldownReason)
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

type autoScanOutcome int

const (
	autoScanOutcomeSkipped autoScanOutcome = iota
	autoScanOutcomeSucceeded
	autoScanOutcomeFailed
)

type autoScanDetail struct {
	outcome    autoScanOutcome
	reason     string
	errMessage string
	err        error
}

func (s autoScanSnapshot) activateCandidate(ctx context.Context, candidate autoCandidate) error {
	return s.activateCandidateDetail(ctx, candidate).err
}

func (s autoScanSnapshot) activateCandidateOutcome(ctx context.Context, candidate autoCandidate) (autoScanOutcome, string) {
	d := s.activateCandidateDetail(ctx, candidate)
	return d.outcome, d.reason
}

func (s autoScanSnapshot) activateCandidateWithOutcome(ctx context.Context, candidate autoCandidate) (autoScanOutcome, string, error) {
	d := s.activateCandidateDetail(ctx, candidate)
	return d.outcome, d.reason, d.err
}

func (s autoScanSnapshot) activateCandidateDetail(ctx context.Context, candidate autoCandidate) autoScanDetail {
	observedAt := s.now().UTC()
	previous := previousStateFromStore(s.store, candidate.authID, candidate.provider)
	decision, err := detector.EvaluateWithPrevious(detector.ProbeInput{
		AuthID: candidate.authID, Provider: candidate.provider, Model: candidate.model,
		ObservedAt: observedAt, Payload: candidate.payload,
	}, previous)
	if err != nil {
		msg := localizeHistoryMessage(err.Error())
		return autoScanDetail{outcome: autoScanOutcomeFailed, reason: msg, errMessage: msg, err: err}
	}
	if !decision.Activate {
		return autoScanDetail{outcome: autoScanOutcomeSkipped, reason: decision.Reason}
	}
	if s.shouldSkipActivateCooldown != nil && s.shouldSkipActivateCooldown(candidate.authID) {
		reason := schedulerMissCooldownReason
		if s.cooldownSkipReason != nil {
			if r := s.cooldownSkipReason(candidate.authID); r != "" {
				reason = r
			}
		}
		return autoScanDetail{outcome: autoScanOutcomeSkipped, reason: reason}
	}
	result, err := s.activator.Activate(ctx, activator.Request{
		AuthID: candidate.authID, Provider: candidate.provider,
		ModelGroup: decision.Observation.ModelGroup, Window: decision.Observation.Window,
		CycleKey: decision.CycleKey, Model: candidate.model, Disabled: candidate.disabled,
		ObservedAt: observedAt, ResetAt: decision.Observation.ResetAt,
		Remaining: decision.Observation.Remaining, HasRemaining: decision.Observation.HasRemaining,
	})
	failText := result.LastError
	if failText == "" && err != nil {
		failText = err.Error()
	}
	s.maybeNoteCooldown(candidate.authID, failText)
	if err != nil {
		msg := localizeHistoryMessage(failText)
		if msg == "" {
			msg = localizeHistoryMessage(err.Error())
		}
		return autoScanDetail{outcome: autoScanOutcomeFailed, reason: msg, errMessage: msg, err: err}
	}
	if result.Status == activator.StatusSuccess && result.Success {
		return autoScanDetail{outcome: autoScanOutcomeSucceeded}
	}
	if result.Status == activator.StatusSkipped || result.Status == activator.StatusBusy {
		reason := decision.Reason
		if reason == "" {
			reason = string(result.Status)
		}
		if result.LastError != "" {
			reason = result.LastError
		}
		return autoScanDetail{outcome: autoScanOutcomeSkipped, reason: reason}
	}
	msg := localizeHistoryMessage(result.LastError)
	if msg == "" {
		msg = "唤醒失败"
	}
	return autoScanDetail{outcome: autoScanOutcomeFailed, reason: msg, errMessage: msg}
}

func (s autoScanSnapshot) maybeNoteCooldown(authID, failText string) {
	if s.noteActivateCooldown == nil || failText == "" {
		return
	}
	switch {
	case isSchedulerMissError(failText):
		s.noteActivateCooldown(authID, schedulerMissCooldownReason)
	case isHostExecutorUnavailableError(failText):
		s.noteActivateCooldown(authID, hostExecutorUnavailableCooldownReason)
	}
}

func isSchedulerMissError(message string) bool {
	if message == "" {
		return false
	}
	if message == schedulerMissErrorMessage || message == schedulerMissCooldownReason {
		return true
	}
	lower := strings.ToLower(message)
	return strings.Contains(message, "调度器未选中目标凭证") ||
		strings.Contains(lower, "activation scheduler did not select")
}

func isHostExecutorUnavailableError(message string) bool {
	if message == "" {
		return false
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "host model executor is unavailable") {
		return true
	}
	if strings.Contains(lower, "host_call_failed") && strings.Contains(lower, "executor") {
		return true
	}
	if strings.Contains(lower, "host callback host.model.execute") && strings.Contains(lower, "unavailable") {
		return true
	}
	return strings.Contains(message, hostExecutorUnavailableMessage) ||
		strings.Contains(message, hostExecutorUnavailableCooldownReason)
}

func previousStateFromStore(store *state.Store, authID string, provider detector.Provider) detector.PreviousState {
	if store == nil {
		return detector.PreviousState{}
	}
	cycleKey, remaining, hasRemaining, ok := store.LatestSuccessCycle(authID, string(provider))
	if !ok {
		return detector.PreviousState{}
	}
	if !hasRemaining && remaining != 0 {
		hasRemaining = true
	}
	return detector.PreviousState{
		CycleKey: cycleKey, Remaining: remaining, HasRemaining: hasRemaining,
	}
}
