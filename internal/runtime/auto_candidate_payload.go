package runtime

import (
	"encoding/json"
	"strings"

	"quota-activation/internal/host"
)

func autoQuotaPayload(file host.AuthFile, document autoAuthFileDocument) ([]byte, bool) {
	for _, raw := range []json.RawMessage{document.QuotaPayload, document.QuotaPayloadUpper} {
		if len(raw) > 0 {
			return append([]byte(nil), raw...), true
		}
	}
	for _, values := range []map[string]any{file.Metadata, file.Attributes} {
		for _, key := range []string{"quota_payload", "quotaPayload"} {
			encoded, ok := encodeAutoPayload(values[key])
			if ok {
				return encoded, true
			}
		}
	}
	return nil, false
}

func encodeAutoPayload(raw any) ([]byte, bool) {
	switch value := raw.(type) {
	case nil:
		return nil, false
	case json.RawMessage:
		return append([]byte(nil), value...), len(value) > 0
	case []byte:
		return append([]byte(nil), value...), len(value) > 0
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil, false
		}
		return []byte(trimmed), true
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, false
		}
		return encoded, true
	}
}

func (d autoAuthFileDocument) availableModels() []string {
	models := append([]string(nil), d.AvailableModels...)
	models = append(models, d.AvailableModelsUpper...)
	models = append(models, d.Models...)
	models = append(models, d.RecentModels...)
	return uniqueAutoModels(models)
}

func autoRuntimeModels(file host.AuthFile) []string {
	models := append([]string(nil), file.Models...)
	models = append(models, file.RecentModels...)
	models = append(models, autoStringSliceFromMap(file.Metadata, "available_models")...)
	models = append(models, autoStringSliceFromMap(file.Metadata, "availableModels")...)
	models = append(models, autoStringSliceFromMap(file.Metadata, "models")...)
	models = append(models, autoStringSliceFromMap(file.Attributes, "available_models")...)
	models = append(models, autoStringSliceFromMap(file.Attributes, "availableModels")...)
	models = append(models, autoStringSliceFromMap(file.Attributes, "models")...)
	return uniqueAutoModels(models)
}

func autoStringSliceFromMap(values map[string]any, key string) []string {
	if len(values) == 0 {
		return nil
	}
	switch raw := values[key].(type) {
	case []string:
		return raw
	case []any:
		items := make([]string, 0, len(raw))
		for _, item := range raw {
			if value, ok := item.(string); ok {
				items = append(items, value)
			}
		}
		return items
	case string:
		return strings.Split(raw, ",")
	default:
		return nil
	}
}

func uniqueAutoModels(models []string) []string {
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

func hasAutoModel(models []string, target string) bool {
	for _, model := range models {
		if strings.TrimSpace(model) == target {
			return true
		}
	}
	return false
}
