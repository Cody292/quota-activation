package management

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"quota-activation/internal/activator"
	"quota-activation/internal/config"
	"quota-activation/internal/detector"
)

type activationRequest struct {
	AuthID           string          `json:"auth_id"`
	Provider         string          `json:"provider"`
	ModelGroup       string          `json:"model_group"`
	Model            string          `json:"model"`
	Prompt           string          `json:"prompt"`
	PreviousCycleKey string          `json:"previous_cycle_key"`
	Disabled         bool            `json:"disabled"`
	QuotaPayload     json.RawMessage `json:"quota_payload"`
}

func decodeActivationRequest(request *http.Request) (activationRequest, error) {
	defer request.Body.Close()
	limited := io.LimitReader(request.Body, 1<<20)
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	var decoded activationRequest
	if err := decoder.Decode(&decoded); err != nil {
		return activationRequest{}, fmt.Errorf("decode activation request: %w", err)
	}
	decoded.AuthID = strings.TrimSpace(decoded.AuthID)
	decoded.Provider = strings.TrimSpace(decoded.Provider)
	decoded.ModelGroup = strings.TrimSpace(decoded.ModelGroup)
	decoded.Model = strings.TrimSpace(decoded.Model)
	decoded.Prompt = strings.TrimSpace(decoded.Prompt)
	decoded.PreviousCycleKey = strings.TrimSpace(decoded.PreviousCycleKey)
	if decoded.AuthID == "" {
		return activationRequest{}, fmt.Errorf("auth_id is required")
	}
	if len(decoded.QuotaPayload) == 0 || strings.TrimSpace(string(decoded.QuotaPayload)) == "" {
		return activationRequest{}, fmt.Errorf("quota_payload is required")
	}
	return decoded, nil
}

func (r activationRequest) toActivatorRequest(cfg config.Config, observedAt time.Time) (activator.Request, error) {
	provider, err := parseProvider(r.Provider)
	if err != nil {
		return activator.Request{}, err
	}
	modelGroup, err := parseModelGroup(provider, r.ModelGroup, r.Model)
	if err != nil {
		return activator.Request{}, err
	}
	if err := ensureModelGroupEnabled(cfg, modelGroup); err != nil {
		return activator.Request{}, err
	}
	model := modelForProvider(cfg, provider, modelGroup, r.Model)
	decision, err := detector.Evaluate(detector.ProbeInput{
		AuthID:     r.AuthID,
		Provider:   provider,
		Model:      model,
		ObservedAt: observedAt,
		Payload:    r.QuotaPayload,
	}, "")
	if err != nil {
		return activator.Request{}, fmt.Errorf("evaluate quota: %w", err)
	}
	if !decision.Activate {
		return activator.Request{}, fmt.Errorf("quota cycle already handled")
	}
	return activator.Request{
		AuthID:     r.AuthID,
		Provider:   provider,
		ModelGroup: modelGroup,
		Window:     decision.Observation.Window,
		CycleKey:   decision.CycleKey,
		Model:      model,
		Prompt:     r.Prompt,
		Disabled:   r.Disabled,
		ObservedAt: observedAt,
		ResetAt:    decision.Observation.ResetAt,
	}, nil
}

func ensureModelGroupEnabled(cfg config.Config, modelGroup detector.ModelGroup) error {
	switch modelGroup {
	case detector.ModelGroupGemini:
		if !cfg.ActivationModels.Antigravity.EnableGemini {
			return fmt.Errorf("antigravity model_group gemini is disabled")
		}
	case detector.ModelGroupClaudeGPT:
		if !cfg.ActivationModels.Antigravity.EnableClaudeGPT {
			return fmt.Errorf("antigravity model_group claude_gpt is disabled")
		}
	case detector.ModelGroupNone:
		return nil
	default:
		return fmt.Errorf("model_group is unsupported")
	}
	return nil
}

func parseProvider(raw string) (detector.Provider, error) {
	switch detector.Provider(strings.TrimSpace(raw)) {
	case detector.ProviderCodex:
		return detector.ProviderCodex, nil
	case detector.ProviderAntigravity:
		return detector.ProviderAntigravity, nil
	case detector.ProviderUnknown:
		return detector.ProviderUnknown, fmt.Errorf("provider is required")
	default:
		return detector.ProviderUnknown, fmt.Errorf("provider is unsupported")
	}
}

func parseModelGroup(provider detector.Provider, raw string, requestedModel string) (detector.ModelGroup, error) {
	trimmed := strings.TrimSpace(raw)
	if provider != detector.ProviderAntigravity {
		return detector.ModelGroupNone, nil
	}
	if trimmed == "" {
		if strings.TrimSpace(requestedModel) != "" {
			return detector.ModelGroupNone, nil
		}
		return detector.ModelGroupNone, fmt.Errorf("model_group is required for antigravity when model is empty")
	}
	switch detector.ModelGroup(trimmed) {
	case detector.ModelGroupGemini:
		return detector.ModelGroupGemini, nil
	case detector.ModelGroupClaudeGPT:
		return detector.ModelGroupClaudeGPT, nil
	case detector.ModelGroupNone:
		return detector.ModelGroupNone, fmt.Errorf("model_group is required for antigravity when model is empty")
	default:
		return detector.ModelGroupNone, fmt.Errorf("model_group is unsupported")
	}
}

func modelForProvider(cfg config.Config, provider detector.Provider, modelGroup detector.ModelGroup, requested string) string {
	if strings.TrimSpace(requested) != "" {
		return strings.TrimSpace(requested)
	}
	switch provider {
	case detector.ProviderCodex:
		return strings.TrimSpace(cfg.ActivationModels.Codex)
	case detector.ProviderAntigravity:
		switch modelGroup {
		case detector.ModelGroupGemini:
			return strings.TrimSpace(cfg.ActivationModels.Antigravity.Gemini)
		case detector.ModelGroupClaudeGPT:
			return strings.TrimSpace(cfg.ActivationModels.Antigravity.ClaudeGPT)
		case detector.ModelGroupNone:
			return ""
		default:
			return ""
		}
	case detector.ProviderUnknown:
		return ""
	default:
		return ""
	}
}
