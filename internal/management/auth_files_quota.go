package management

import (
	"encoding/json"
	"time"

	"quota-activation/internal/detector"
	"quota-activation/internal/host"
	"quota-activation/internal/quotapayload"
)

func (h *Handler) quotaPayload(file host.AuthFile, provider string, availableModels []string, authID string) any {
	if payload, ok := quotapayload.TrueFromFile(file); ok {
		return json.RawMessage(payload)
	}
	now := h.now().UTC()
	if record, ok := h.store.LatestSuccess(authID, provider); ok {
		if name, limitSeconds, resetAt, ok := quotapayload.WindowFromSuccess(record, now); ok {
			if assembled, ok := assembleQuotaPayload(provider, availableModels, name, limitSeconds, resetAt); ok {
				return assembled
			}
		}
	}
	switch detector.Provider(provider) {
	case detector.ProviderCodex:
		return map[string]any{"reset_after_seconds": 18000, "limit_window_seconds": 18000, "name": "5h"}
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

func assembleQuotaPayload(provider string, availableModels []string, windowName string, limitSeconds int, resetAt time.Time) (any, bool) {
	raw, ok := quotapayload.Marshal(detector.Provider(provider), availableModels, windowName, limitSeconds, resetAt)
	if !ok {
		return nil, false
	}
	return json.RawMessage(raw), true
}
