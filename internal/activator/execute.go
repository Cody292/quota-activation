package activator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"quota-activation/internal/detector"
	"quota-activation/internal/host"
	"quota-activation/internal/scheduler"
	"quota-activation/internal/session"
)

func (a *Activator) execute(ctx context.Context, request Request, result Result) (Result, error) {
	callCtx := ctx
	cancel := func() {}
	if a.config.ActivationRequestTimeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, a.config.ActivationRequestTimeout)
	}
	defer cancel()
	created, err := a.sessions.Create(session.Target{AuthID: request.AuthID, Provider: string(request.Provider), Window: string(request.Window)})
	if err != nil {
		return result, fmt.Errorf("create activation session: %w", safeError(err))
	}
	model, err := a.availableModel(callCtx, request)
	if err != nil {
		return result, err
	}
	request.Model = model
	body, err := json.Marshal(activationPingBody(request))
	if err != nil {
		return result, fmt.Errorf("encode activation ping: %w", safeError(err))
	}
	executeRequest := host.ModelExecuteRequest{
		Model:   request.Model,
		Stream:  false,
		Body:    body,
		Headers: map[string][]string{scheduler.NonceHeaderName: {created.Nonce}},
	}
	if request.Provider == detector.ProviderAntigravity {
		executeRequest.EntryProtocol = "openai"
		executeRequest.ExitProtocol = "openai"
	}
	response, err := a.host.ModelExecute(callCtx, executeRequest)
	result.Nonce = created.Nonce
	if err != nil {
		result.HTTPStatus = statusCodeFromError(err)
		if request.Provider == detector.ProviderCodex {
			message, ok := usageLimitBusinessMessage(err)
			if !ok {
				return result, fmt.Errorf("host model execute: %w", safeHostError(err))
			}
			result.Status = StatusFailed
			result.Success = false
			result.LastError = message
			return result, nil
		}
		return result, fmt.Errorf("host model execute: %w", safeHostError(err))
	}
	checked, err := host.ResponseOrStatusError(response)
	if err != nil {
		result.HTTPStatus = response.StatusCode
		return result, fmt.Errorf("host model execute: %w", safeHostError(err))
	}
	result.HTTPStatus = checked.StatusCode
	if !a.activationSessionConsumed(created.Nonce) {
		result.Status = StatusFailed
		result.Success = false
		result.LastError = "调度器未选中目标凭证"
		return result, nil
	}
	if request.Provider == detector.ProviderAntigravity && !antigravityActivationSucceeded(checked) {
		result.Status = StatusFailed
		result.Success = false
		result.LastError = "Antigravity 唤醒响应缺少 choices"
		return result, nil
	}
	return result, nil
}

type authFileModelsDocument struct {
	AuthID               string   `json:"auth_id"`
	AuthIDUpper          string   `json:"authID"`
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	AvailableModels      []string `json:"available_models"`
	AvailableModelsUpper []string `json:"availableModels"`
	Models               []string `json:"models"`
	RecentModels         []string `json:"recent_models"`
}

func (a *Activator) availableModel(ctx context.Context, request Request) (string, error) {
	if requestedModel := strings.TrimSpace(request.Model); requestedModel != "" {
		return requestedModel, nil
	}
	files, err := a.host.ListAuthFiles(ctx)
	if err != nil {
		return "", fmt.Errorf("list auth files for available models: %w", safeHostError(err))
	}
	for _, file := range files {
		if file.AuthIndex != "" {
			if runtimeFile, err := a.host.GetRuntimeAuthFile(ctx, file.AuthIndex); err == nil {
				file = mergeRuntimeModelFile(file, runtimeFile)
			}
		}
		document := decodeAuthFileModels(file)
		if request.AuthID != firstNonBlankModelField(file.ID, file.Name, document.Name, file.AuthIndex, document.AuthID, document.AuthIDUpper, document.ID) {
			continue
		}
		availableModels := uniqueActivationModels(append(runtimeModelFields(file), document.availableModels()...))
		requestedModel := strings.TrimSpace(request.Model)
		if requestedModel != "" {
			for _, model := range availableModels {
				if model == requestedModel {
					return model, nil
				}
			}
		}
		for _, model := range availableModels {
			if request.Provider == detector.ProviderAntigravity && request.ModelGroup != detector.ModelGroupNone && antigravityModelGroup(model) != request.ModelGroup {
				continue
			}
			return model, nil
		}
		return "", fmt.Errorf("available model is required for auth %s", request.AuthID)
	}
	return "", fmt.Errorf("available model is required for auth %s", request.AuthID)
}

func (a *Activator) activationSessionConsumed(nonce string) bool {
	if a == nil || a.sessions == nil {
		return false
	}
	_, err := a.sessions.Lookup(nonce)
	return errors.Is(err, session.ErrSessionNotFound)
}

func mergeRuntimeModelFile(base host.AuthFile, runtime host.AuthFile) host.AuthFile {
	if runtime.ID == "" {
		runtime.ID = base.ID
	}
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
	if runtime.Data == nil {
		runtime.Data = base.Data
	}
	return runtime
}

func firstNonBlankModelField(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func decodeAuthFileModels(file host.AuthFile) authFileModelsDocument {
	var document authFileModelsDocument
	if err := json.Unmarshal(file.Data, &document); err != nil {
		return authFileModelsDocument{Name: file.Name}
	}
	return document
}

func (d authFileModelsDocument) availableModels() []string {
	models := append([]string(nil), d.AvailableModels...)
	models = append(models, d.AvailableModelsUpper...)
	models = append(models, d.Models...)
	models = append(models, d.RecentModels...)
	return uniqueActivationModels(models)
}

func runtimeModelFields(file host.AuthFile) []string {
	models := append([]string(nil), file.Models...)
	models = append(models, file.RecentModels...)
	models = append(models, activationStringSliceFromMap(file.Metadata, "available_models")...)
	models = append(models, activationStringSliceFromMap(file.Metadata, "availableModels")...)
	models = append(models, activationStringSliceFromMap(file.Metadata, "models")...)
	models = append(models, activationStringSliceFromMap(file.Attributes, "available_models")...)
	models = append(models, activationStringSliceFromMap(file.Attributes, "availableModels")...)
	models = append(models, activationStringSliceFromMap(file.Attributes, "models")...)
	return models
}

func uniqueActivationModels(models []string) []string {
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

func activationStringSliceFromMap(values map[string]any, key string) []string {
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

const usageLimitReachedType = "usage_limit_reached"

type hostCallbackErrorPayload struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type antigravityActivationResponse struct {
	Choices []json.RawMessage `json:"choices"`
	Error   *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func usageLimitBusinessMessage(err error) (string, bool) {
	text := err.Error()
	if !strings.Contains(text, usageLimitReachedType) {
		return "", false
	}
	message := "The usage limit has been reached"
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		var payload hostCallbackErrorPayload
		if json.Unmarshal([]byte(text[start:end+1]), &payload) == nil && payload.Error.Type == usageLimitReachedType {
			if parsed := strings.TrimSpace(payload.Error.Message); parsed != "" {
				message = parsed
			}
		}
	}
	return usageLimitReachedType + ": " + message, true
}

func antigravityActivationSucceeded(response host.ModelExecuteResponse) bool {
	var payload antigravityActivationResponse
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return false
	}
	return payload.Error == nil && len(payload.Choices) > 0
}

type codexPing struct {
	Model string         `json:"model"`
	Input []modelMessage `json:"input"`
	Store bool           `json:"store"`
}

type modelPing struct {
	Model    string         `json:"model"`
	Messages []modelMessage `json:"messages"`
}

type legacyPing struct {
	Model    string         `json:"model"`
	Prompt   string         `json:"prompt"`
	Messages []modelMessage `json:"messages"`
}

type modelMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func activationPingBody(request Request) any {
	if request.Provider == detector.ProviderCodex {
		return codexPing{Model: request.Model, Input: []modelMessage{{Role: "user", Content: request.Prompt}}, Store: false}
	}
	if request.Provider != detector.ProviderAntigravity {
		return legacyPing{
			Model:    request.Model,
			Prompt:   request.Prompt,
			Messages: []modelMessage{{Role: "user", Content: request.Prompt}},
		}
	}
	return modelPing{
		Model:    request.Model,
		Messages: []modelMessage{{Role: "user", Content: request.Prompt}},
	}
}

func statusCodeFromError(err error) int {
	var statusErr *host.HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode
	}
	if isNetworkFailure(err) {
		return http.StatusBadGateway
	}
	return 0
}
