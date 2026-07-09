package management

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"quota-activation/internal/detector"
	"quota-activation/internal/host"
)

type authFilesResponse struct {
	Files []authFileChoice `json:"files"`
}

type authFileChoice struct {
	AuthID       string        `json:"auth_id"`
	Provider     string        `json:"provider"`
	Disabled     bool          `json:"disabled"`
	Models       []modelChoice `json:"models"`
	QuotaPayload any           `json:"quota_payload"`
}

type modelChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Group string `json:"group,omitempty"`
}

type authFileDocument struct {
	AuthID      string `json:"auth_id"`
	AuthIDUpper string `json:"authID"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Type        string `json:"type"`
	Disabled    bool   `json:"disabled"`
}

func (h *Handler) handleAuthFiles(w http.ResponseWriter, request *http.Request) {
	if h.host == nil {
		writeJSON(w, http.StatusOK, authFilesResponse{Files: []authFileChoice{}})
		return
	}
	files, err := h.host.ListAuthFiles(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "auth_files_failed", err.Error())
		return
	}
	choices := make([]authFileChoice, 0, len(files))
	for _, file := range files {
		choice, ok := h.authFileChoice(file)
		if ok {
			choices = append(choices, choice)
		}
	}
	writeJSON(w, http.StatusOK, authFilesResponse{Files: choices})
}

func (h *Handler) authFileChoice(file host.AuthFile) (authFileChoice, bool) {
	document := decodeAuthFile(file)
	authID := firstNonBlank(file.AuthIndex, document.AuthID, document.AuthIDUpper, document.ID, document.Name, file.Name)
	provider := normalizeProvider(firstNonBlank(file.Provider, file.Type, document.Provider, document.Type))
	if authID == "" || provider == "" {
		return authFileChoice{}, false
	}
	return authFileChoice{AuthID: authID, Provider: provider, Disabled: file.Disabled || document.Disabled, Models: h.modelChoices(provider), QuotaPayload: h.quotaPayload(provider)}, true
}

func decodeAuthFile(file host.AuthFile) authFileDocument {
	var document authFileDocument
	if err := json.Unmarshal(file.Data, &document); err != nil {
		return authFileDocument{Name: file.Name}
	}
	return document
}

func normalizeProvider(raw string) string {
	text := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case text == string(detector.ProviderCodex), strings.Contains(text, "codex"):
		return string(detector.ProviderCodex)
	case text == string(detector.ProviderAntigravity), strings.Contains(text, "antigravity"):
		return string(detector.ProviderAntigravity)
	default:
		return ""
	}
}

func (h *Handler) modelChoices(provider string) []modelChoice {
	switch detector.Provider(provider) {
	case detector.ProviderCodex:
		return []modelChoice{{Value: h.config.ActivationModels.Codex, Label: fmt.Sprintf("Codex · %s", h.config.ActivationModels.Codex)}}
	case detector.ProviderAntigravity:
		choices := []modelChoice{}
		if h.config.ActivationModels.Antigravity.EnableClaudeGPT {
			model := h.config.ActivationModels.Antigravity.ClaudeGPT
			choices = append(choices, modelChoice{Value: model, Label: fmt.Sprintf("Claude/GPT · %s", model), Group: string(detector.ModelGroupClaudeGPT)})
		}
		if h.config.ActivationModels.Antigravity.EnableGemini {
			model := h.config.ActivationModels.Antigravity.Gemini
			choices = append(choices, modelChoice{Value: model, Label: fmt.Sprintf("Gemini · %s", model), Group: string(detector.ModelGroupGemini)})
		}
		return choices
	default:
		return nil
	}
}

func (h *Handler) quotaPayload(provider string) any {
	resetAt := h.now().UTC().Add(5 * time.Hour).Format(time.RFC3339)
	switch detector.Provider(provider) {
	case detector.ProviderCodex:
		return map[string]any{"reset_after_seconds": 18000, "limit_window_seconds": 18000, "name": "5h"}
	case detector.ProviderAntigravity:
		return map[string]any{
			"models": map[string]any{
				h.config.ActivationModels.Antigravity.ClaudeGPT: map[string]any{"modelProvider": "anthropic", "quotaInfo": map[string]any{"windows": []map[string]any{{"resetTime": resetAt, "name": "5h"}}}},
				h.config.ActivationModels.Antigravity.Gemini:    map[string]any{"modelProvider": "gemini", "quotaInfo": map[string]any{"windows": []map[string]any{{"resetTime": resetAt, "name": "5h"}}}},
			},
		}
	default:
		return map[string]any{}
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
