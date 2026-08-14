package runtime

import (
	"strings"

	"quota-activation/internal/host"
	"quota-activation/internal/quotapayload"
)

func autoQuotaPayload(file host.AuthFile, _ autoAuthFileDocument) ([]byte, bool) {
	return quotapayload.TrueFromFile(file)
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
