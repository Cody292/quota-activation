package management

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"quota-activation/internal/detector"
	"quota-activation/internal/host"
	"quota-activation/internal/planclaim"
	"quota-activation/internal/quotapayload"
	"quota-activation/internal/state"
)

func (h *Handler) quotaPayload(file host.AuthFile, provider string, availableModels []string, authID string) any {
	now := time.Now().UTC()
	if h != nil && h.now != nil {
		now = h.now().UTC()
	}
	if payload, ok := quotapayload.TrueFromFile(file); ok {
		if sanitized, ok := quotapayload.SanitizeForProvider(detector.Provider(provider), payload); ok && !codexFiveHourOnly(provider, authID, sanitized, now) {
			return json.RawMessage(sanitized)
		}
	}
	if detector.Provider(provider) == detector.ProviderAntigravity {
		assembled := map[string]any{"models": map[string]any{}}
		models := assembled["models"].(map[string]any)
		var store *state.Store
		if h != nil {
			store = h.store
		}
		hasGroupHistory := h.hasActiveNamedGroupHistory(authID, provider, now)
		for _, model := range availableModels {
			name, seconds, resetAt, ok := quotapayload.SyntheticWindow(detector.ProviderAntigravity, now)
			if group, grouped := detector.InferModelGroup(model); grouped {
				if record, found := store.UsableLatestSuccessForModelGroup(authID, provider, string(group)); found {
					if n, s, r, valid := quotapayload.WindowFromSuccess(record, now); valid {
						name, seconds, resetAt, ok = n, s, r, true
					}
				} else if hasGroupHistory {
					continue
				}
			}
			if ok {
				if payload, valid := quotapayload.MarshalAntigravity([]string{model}, name, seconds, resetAt); valid {
					for key, value := range payload["models"].(map[string]any) {
						models[key] = value
					}
				}
			}
		}
		return assembled
	}
	if record, ok := h.store.UsableLatestSuccess(authID, provider); ok {
		if name, limitSeconds, resetAt, ok := quotapayload.WindowFromSuccess(record, now); ok {
			if assembled, ok := assembleQuotaPayload(provider, availableModels, name, limitSeconds, resetAt); ok {
				return assembled
			}
		}
	}
	switch detector.Provider(provider) {
	case detector.ProviderCodex:
		name, limitSeconds, resetAt, ok := quotapayload.CodexWindowForPlan(h.codexPlan(file, authID), now)
		if !ok {
			return map[string]any{}
		}
		assembled, ok := assembleQuotaPayload(provider, availableModels, name, limitSeconds, resetAt)
		if !ok {
			return map[string]any{}
		}
		return assembled
	case detector.ProviderAntigravity:
		name, limitSeconds, resetAt, ok := quotapayload.SyntheticWindow(detector.ProviderAntigravity, now)
		if !ok {
			return map[string]any{}
		}
		assembled, ok := quotapayload.MarshalAntigravity(availableModels, name, limitSeconds, resetAt)
		if !ok {
			return map[string]any{"models": map[string]any{}}
		}
		return assembled
	default:
		return map[string]any{}
	}
}

func (h *Handler) hasActiveNamedGroupHistory(authID string, provider string, now time.Time) bool {
	if h == nil || h.store == nil {
		return false
	}
	wantAuthID := state.Redact(strings.TrimSpace(authID))
	wantProvider := state.Redact(strings.TrimSpace(provider))
	if wantAuthID == "" || wantProvider == "" {
		return false
	}
	for _, record := range h.store.SnapshotUsableSuccess() {
		if record.AuthID != wantAuthID || record.Provider != wantProvider {
			continue
		}
		if state.ModelGroupOf(record) == "" {
			continue
		}
		if _, _, _, valid := quotapayload.WindowFromSuccess(record, now); valid {
			return true
		}
	}
	return false
}

// codexFiveHourOnly 将 Codex 仅 5h 的真源视为缺失（与 parseCodex 忽略 5h 一致）。
func codexFiveHourOnly(provider string, authID string, payload json.RawMessage, now time.Time) bool {
	if detector.Provider(provider) != detector.ProviderCodex {
		return false
	}
	_, err := detector.Evaluate(detector.ProbeInput{
		AuthID:     authID,
		Provider:   detector.ProviderCodex,
		ObservedAt: now,
		Payload:    payload,
	}, "")
	return err != nil
}

// codexPlan 从物理 JSON 解析套餐；List stub 无 JWT 时补拉 host.GetAuthFile。
func (h *Handler) codexPlan(file host.AuthFile, authID string) planclaim.Type {
	plan := planclaim.FromAuthJSON(file.Data)
	if plan != planclaim.TypeUnknown || h.host == nil {
		return plan
	}
	for _, key := range []string{file.AuthIndex, file.Name, file.ID, authID} {
		if key == "" {
			continue
		}
		physical, err := h.host.GetAuthFile(context.Background(), key)
		if err != nil {
			continue
		}
		if got := planclaim.FromAuthJSON(physical.Data); got != planclaim.TypeUnknown {
			return got
		}
	}
	return planclaim.TypeUnknown
}

func assembleQuotaPayload(provider string, availableModels []string, windowName string, limitSeconds int, resetAt time.Time) (any, bool) {
	raw, ok := quotapayload.Marshal(detector.Provider(provider), availableModels, windowName, limitSeconds, resetAt)
	if !ok {
		return nil, false
	}
	return json.RawMessage(raw), true
}
