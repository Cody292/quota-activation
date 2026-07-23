package runtime

import (
	"encoding/json"
	"strings"
	"time"

	"quota-activation/internal/config"
	"quota-activation/internal/detector"
	"quota-activation/internal/host"
	"quota-activation/internal/state"
)

type autoAuthFileDocument struct {
	AuthID               string          `json:"auth_id"`
	AuthIDUpper          string          `json:"authID"`
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	Provider             string          `json:"provider"`
	Type                 string          `json:"type"`
	Disabled             bool            `json:"disabled"`
	QuotaPayload         json.RawMessage `json:"quota_payload"`
	QuotaPayloadUpper    json.RawMessage `json:"quotaPayload"`
	AvailableModels      []string        `json:"available_models"`
	AvailableModelsUpper []string        `json:"availableModels"`
	Models               []string        `json:"models"`
	RecentModels         []string        `json:"recent_models"`
}

type autoRuntimeAuthFile struct {
	file host.AuthFile
	ok   bool
}

func (s autoScanSnapshot) autoCandidate(file host.AuthFile, runtimeAuth autoRuntimeAuthFile) (autoCandidate, bool) {
	document := decodeAutoAuthFile(file)
	// 宿主 scheduler 候选 Auth.ID 与 host.auth.list 的 id/name（文件名）一致；auth_index 哈希不得优先，否则 pick 命中失败。
	authID := firstNonBlankAuto(file.ID, file.Name, document.Name, document.ID, document.AuthID, document.AuthIDUpper, file.AuthIndex)
	provider, ok := autoProvider(firstNonBlankAuto(file.Provider, file.Type, document.Provider, document.Type))
	if !ok || authID == "" {
		return autoCandidate{}, false
	}
	disabled := file.Disabled || document.Disabled
	if disabled && !s.config.EnableBeforeActivation {
		return autoCandidate{}, false
	}
	model, ok := autoModel(s.config, provider, autoCandidateModels(file, document, runtimeAuth))
	if !ok {
		return autoCandidate{}, false
	}
	payload, ok := autoCandidatePayload(file, document, runtimeAuth, provider, model, authID, s.store, s.now().UTC())
	if !ok {
		return autoCandidate{}, false
	}
	return autoCandidate{authID: authID, provider: provider, model: model, disabled: disabled, payload: payload}, true
}

func autoCandidatePayload(file host.AuthFile, document autoAuthFileDocument, runtimeAuth autoRuntimeAuthFile, provider detector.Provider, model string, authID string, store *state.Store, observedAt time.Time) ([]byte, bool) {
	if runtimeAuth.ok {
		runtimeDocument := decodeAutoAuthFile(runtimeAuth.file)
		if payload, ok := autoQuotaPayload(runtimeAuth.file, runtimeDocument); ok {
			return payload, true
		}
	}
	if payload, ok := autoQuotaPayload(file, document); ok {
		return payload, true
	}
	if store != nil {
		if record, ok := store.LatestSuccess(authID, string(provider)); ok {
			if payload, ok := syntheticInferredQuotaPayload(provider, model, record, observedAt); ok {
				return payload, true
			}
		}
	}
	return syntheticAutoQuotaPayload(provider, model, observedAt)
}

func autoCandidateModels(file host.AuthFile, document autoAuthFileDocument, runtimeAuth autoRuntimeAuthFile) []string {
	models := make([]string, 0)
	if runtimeAuth.ok {
		runtimeDocument := decodeAutoAuthFile(runtimeAuth.file)
		models = append(models, autoRuntimeModels(runtimeAuth.file)...)
		models = append(models, runtimeDocument.availableModels()...)
	}
	models = append(models, autoRuntimeModels(file)...)
	models = append(models, document.availableModels()...)
	return models
}

func decodeAutoAuthFile(file host.AuthFile) autoAuthFileDocument {
	var document autoAuthFileDocument
	if err := json.Unmarshal(file.Data, &document); err != nil {
		return autoAuthFileDocument{Name: file.Name}
	}
	return document
}

func autoProvider(raw string) (detector.Provider, bool) {
	text := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case text == string(detector.ProviderCodex), strings.Contains(text, "codex"):
		return detector.ProviderCodex, true
	case text == string(detector.ProviderAntigravity), strings.Contains(text, "antigravity"):
		return detector.ProviderAntigravity, true
	default:
		return detector.ProviderUnknown, false
	}
}

func autoModel(cfg config.Config, provider detector.Provider, availableModels []string) (string, bool) {
	switch provider {
	case detector.ProviderCodex:
		model := strings.TrimSpace(cfg.ActivationModels.Codex.Models)
		if model == "" {
			return "", false
		}
		// CPA host.auth.list/get_runtime 通常不返回 models；有列表时做存在性校验，无列表时直接使用配置模型。
		if len(availableModels) > 0 && !hasAutoModel(availableModels, model) {
			return "", false
		}
		return model, true
	case detector.ProviderAntigravity:
		return autoAntigravityModel(cfg, availableModels)
	case detector.ProviderUnknown:
		return "", false
	default:
		return "", false
	}
}

func autoAntigravityModel(cfg config.Config, availableModels []string) (string, bool) {
	model := strings.TrimSpace(cfg.ActivationModels.Antigravity.Models)
	if model == "" {
		return "", false
	}
	if group := strings.TrimSpace(cfg.ActivationModels.Antigravity.ModelsGroup); group != "" {
		inferred, ok := antigravityConfigModelGroup(model)
		if !ok || string(inferred) != group {
			// 模型名无法推断组时，以配置 models_group 为准；可推断时要求与 models_group 一致。
			if ok {
				return "", false
			}
		}
	}
	if len(availableModels) > 0 && !hasAutoModel(availableModels, model) {
		return "", false
	}
	return model, true
}

func antigravityConfigModelGroup(model string) (detector.ModelGroup, bool) {
	lower := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(lower, "gemini") {
		return detector.ModelGroupGemini, true
	}
	if strings.Contains(lower, "claude") || strings.Contains(lower, "gpt") {
		return detector.ModelGroupClaudeGPT, true
	}
	return detector.ModelGroupNone, false
}

func syntheticInferredQuotaPayload(provider detector.Provider, model string, record state.Record, observedAt time.Time) ([]byte, bool) {
	windowName, limitSeconds, ok := normalizeInferredWindow(record.Window)
	if !ok {
		return nil, false
	}
	resetAt := inferredResetAt(record.ResetAt, observedAt, time.Duration(limitSeconds)*time.Second)
	return marshalAutoQuotaPayload(provider, model, windowName, limitSeconds, resetAt)
}

func syntheticAutoQuotaPayload(provider detector.Provider, model string, observedAt time.Time) ([]byte, bool) {
	switch provider {
	case detector.ProviderCodex:
		return marshalAutoQuotaPayload(provider, model, "monthly", 30*24*60*60, stableWindowResetAt(observedAt, 30*24*time.Hour))
	case detector.ProviderAntigravity:
		return marshalAutoQuotaPayload(provider, model, "weekly", 7*24*60*60, stableWindowResetAt(observedAt, 7*24*time.Hour))
	default:
		return nil, false
	}
}

func normalizeInferredWindow(raw string) (name string, limitSeconds int, ok bool) {
	switch detector.Window(strings.ToLower(strings.TrimSpace(raw))) {
	case detector.WindowMonthly:
		return "monthly", 30 * 24 * 60 * 60, true
	case detector.WindowWeekly:
		return "weekly", 7 * 24 * 60 * 60, true
	case detector.WindowFiveHour:
		return "5h", 5 * 60 * 60, true
	default:
		text := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case strings.Contains(text, "month") || strings.Contains(text, "30d"):
			return "monthly", 30 * 24 * 60 * 60, true
		case strings.Contains(text, "week") || strings.Contains(text, "7d"):
			return "weekly", 7 * 24 * 60 * 60, true
		case strings.Contains(text, "5h") || strings.Contains(text, "5 hour"):
			return "5h", 5 * 60 * 60, true
		default:
			return "", 0, false
		}
	}
}

func inferredResetAt(previousReset time.Time, observedAt time.Time, window time.Duration) time.Time {
	observedAt = observedAt.UTC()
	if !previousReset.IsZero() && previousReset.UTC().After(observedAt) {
		return previousReset.UTC()
	}
	return stableWindowResetAt(observedAt, window)
}

func marshalAutoQuotaPayload(provider detector.Provider, model string, windowName string, limitSeconds int, resetAt time.Time) ([]byte, bool) {
	resetText := resetAt.UTC().Format(time.RFC3339)
	switch provider {
	case detector.ProviderCodex:
		payload, err := json.Marshal(map[string]any{
			"reset_at":             resetText,
			"limit_window_seconds": limitSeconds,
			"name":                 windowName,
		})
		return payload, err == nil
	case detector.ProviderAntigravity:
		group, ok := antigravityConfigModelGroup(model)
		if !ok {
			return nil, false
		}
		providerName := "anthropic"
		if group == detector.ModelGroupGemini {
			providerName = "gemini"
		}
		payload, err := json.Marshal(map[string]any{
			"models": map[string]any{
				model: map[string]any{
					"modelProvider": providerName,
					"quotaInfo": map[string]any{
						"windows": []map[string]any{{
							"resetTime":            resetText,
							"name":                 windowName,
							"limit_window_seconds": limitSeconds,
						}},
					},
				},
			},
		})
		return payload, err == nil
	default:
		return nil, false
	}
}

func stableWindowResetAt(observedAt time.Time, window time.Duration) time.Time {
	if window <= 0 {
		window = 7 * 24 * time.Hour
	}
	now := observedAt.UTC()
	return now.Truncate(window).Add(window)
}

func firstNonBlankAuto(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
