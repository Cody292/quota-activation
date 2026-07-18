package management

import (
	"context"
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
	Name         string        `json:"name"`
	Label        string        `json:"label"`
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
	AuthID               string   `json:"auth_id"`
	AuthIDUpper          string   `json:"authID"`
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Account              string   `json:"account"`
	Email                string   `json:"email"`
	Provider             string   `json:"provider"`
	Type                 string   `json:"type"`
	Disabled             bool     `json:"disabled"`
	AvailableModels      []string `json:"available_models"`
	AvailableModelsUpper []string `json:"availableModels"`
	Models               []string `json:"models"`
	RecentModels         []string `json:"recent_models"`
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
	runtimeFile := file
	if file.AuthIndex != "" && h.host != nil {
		if resolved, err := h.host.GetRuntimeAuthFile(context.Background(), file.AuthIndex); err == nil {
			runtimeFile = mergeRuntimeAuthFile(file, resolved)
		}
	}
	document := decodeAuthFile(file)
	authID := firstNonBlank(runtimeFile.ID, file.ID, runtimeFile.Name, file.Name, document.Name, runtimeFile.AuthIndex, file.AuthIndex, document.AuthID, document.AuthIDUpper, document.ID)
	label := firstNonBlank(runtimeFile.Account, runtimeFile.Email, file.Account, file.Email, document.Account, document.Email, document.Name, file.Name, authID)
	provider := normalizeProvider(firstNonBlank(runtimeFile.Provider, runtimeFile.Type, file.Provider, file.Type, document.Provider, document.Type))
	if authID == "" || provider == "" {
		return authFileChoice{}, false
	}
	availableModels := append(runtimeAvailableModels(runtimeFile), document.availableModels()...)
	return authFileChoice{AuthID: authID, Name: firstNonBlank(runtimeFile.Name, file.Name, document.Name), Label: label, Provider: provider, Disabled: runtimeFile.Disabled || file.Disabled || document.Disabled, Models: h.modelChoices(provider, availableModels), QuotaPayload: h.quotaPayload(provider, availableModels)}, true
}

func mergeRuntimeAuthFile(base host.AuthFile, runtime host.AuthFile) host.AuthFile {
	if runtime.AuthIndex == "" {
		runtime.AuthIndex = base.AuthIndex
	}
	if runtime.Name == "" {
		runtime.Name = base.Name
	}
	if runtime.Provider == "" {
		runtime.Provider = base.Provider
	}
	if runtime.Type == "" {
		runtime.Type = base.Type
	}
	if runtime.Email == "" {
		runtime.Email = base.Email
	}
	if runtime.Account == "" {
		runtime.Account = base.Account
	}
	if runtime.Data == nil {
		runtime.Data = base.Data
	}
	return runtime
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

func (d authFileDocument) availableModels() []string {
	models := append([]string(nil), d.AvailableModels...)
	models = append(models, d.AvailableModelsUpper...)
	models = append(models, d.Models...)
	models = append(models, d.RecentModels...)
	return uniqueModels(models)
}

func runtimeAvailableModels(file host.AuthFile) []string {
	models := append([]string(nil), file.Models...)
	models = append(models, file.RecentModels...)
	models = append(models, stringSliceFromMap(file.Metadata, "available_models")...)
	models = append(models, stringSliceFromMap(file.Metadata, "availableModels")...)
	models = append(models, stringSliceFromMap(file.Metadata, "models")...)
	models = append(models, stringSliceFromMap(file.Attributes, "available_models")...)
	models = append(models, stringSliceFromMap(file.Attributes, "availableModels")...)
	models = append(models, stringSliceFromMap(file.Attributes, "models")...)
	return uniqueModels(models)
}

func uniqueModels(models []string) []string {
	choices := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		trimmed := strings.TrimSpace(model)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		choices = append(choices, trimmed)
	}
	return choices
}

func stringSliceFromMap(values map[string]any, key string) []string {
	if len(values) == 0 {
		return nil
	}
	switch raw := values[key].(type) {
	case []string:
		return raw
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if value, ok := item.(string); ok {
				out = append(out, value)
			}
		}
		return out
	case string:
		return strings.Split(raw, ",")
	default:
		return nil
	}
}

func (h *Handler) modelChoices(provider string, availableModels []string) []modelChoice {
	switch detector.Provider(provider) {
	case detector.ProviderCodex:
		choices := make([]modelChoice, 0, len(availableModels))
		for _, model := range availableModels {
			choices = append(choices, modelChoice{Value: model, Label: fmt.Sprintf("Codex · %s", model)})
		}
		return choices
	case detector.ProviderAntigravity:
		choices := make([]modelChoice, 0, len(availableModels))
		for _, model := range availableModels {
			group := antigravityModelGroup(model)
			if group == detector.ModelGroupNone {
				continue
			}
			choices = append(choices, modelChoice{Value: model, Label: fmt.Sprintf("%s · %s", antigravityModelGroupLabel(group), model), Group: string(group)})
		}
		return choices
	default:
		return nil
	}
}

func antigravityModelGroup(model string) detector.ModelGroup {
	lower := strings.ToLower(model)
	if strings.Contains(lower, "gemini") {
		return detector.ModelGroupGemini
	}
	if strings.Contains(lower, "claude") || strings.Contains(lower, "gpt") {
		return detector.ModelGroupClaudeGPT
	}
	return detector.ModelGroupNone
}

func antigravityModelGroupLabel(group detector.ModelGroup) string {
	if group == detector.ModelGroupGemini {
		return "Gemini"
	}
	return "Claude/GPT"
}

func (h *Handler) quotaPayload(provider string, availableModels []string) any {
	resetAt := h.now().UTC().Add(5 * time.Hour).Format(time.RFC3339)
	switch detector.Provider(provider) {
	case detector.ProviderCodex:
		return map[string]any{"reset_after_seconds": 18000, "limit_window_seconds": 18000, "name": "5h"}
	case detector.ProviderAntigravity:
		models := map[string]any{}
		for _, model := range availableModels {
			group := antigravityModelGroup(model)
			if group == detector.ModelGroupNone {
				continue
			}
			providerName := "anthropic"
			if group == detector.ModelGroupGemini {
				providerName = "gemini"
			}
			models[model] = map[string]any{"modelProvider": providerName, "quotaInfo": map[string]any{"windows": []map[string]any{{"resetTime": resetAt, "name": "5h"}}}}
		}
		return map[string]any{
			"models": models,
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
