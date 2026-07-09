package config

import (
	"fmt"
	"strconv"
	"strings"
)

func parseYAMLMap(data string) (map[string]any, error) {
	result := map[string]any{}
	activationModels := map[string]any{}
	antigravity := map[string]any{}
	section := yamlSection{}

	for _, line := range strings.Split(data, "\n") {
		item, ok, err := parseYAMLLine(line)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if item.indent == 0 {
			section = yamlSection{name: item.key, indent: item.indent, subIndent: -1}
			if item.key == "activation_models" {
				result[item.key] = activationModels
				continue
			}
			result[item.key] = yamlScalar(item.value)
			continue
		}
		applyActivationModelsYAML(item, &section, activationModels, antigravity)
	}
	return result, nil
}

type yamlSection struct {
	name      string
	sub       string
	indent    int
	subIndent int
}

type yamlLine struct {
	key    string
	value  string
	indent int
}

func parseYAMLLine(line string) (yamlLine, bool, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return yamlLine{}, false, nil
	}
	key, value, ok := strings.Cut(trimmed, ":")
	if !ok {
		return yamlLine{}, false, fmt.Errorf("must use key: value syntax in line: %s", trimmed)
	}
	return yamlLine{key: strings.TrimSpace(key), value: strings.TrimSpace(value), indent: len(line) - len(strings.TrimLeft(line, " "))}, true, nil
}

func applyActivationModelsYAML(item yamlLine, section *yamlSection, activationModels map[string]any, antigravity map[string]any) {
	if section.name != "activation_models" || item.indent <= section.indent {
		return
	}
	if item.key == "antigravity" && item.value == "" {
		section.sub = item.key
		section.subIndent = item.indent
		activationModels[item.key] = antigravity
		return
	}
	if section.sub == "antigravity" && item.indent > section.subIndent {
		antigravity[item.key] = yamlScalar(item.value)
		return
	}
	activationModels[item.key] = yamlScalar(item.value)
}

func yamlScalar(value string) any {
	text := yamlText(value)
	if parsed, err := strconv.Atoi(text); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseBool(text); err == nil {
		return parsed
	}
	return text
}

func yamlText(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > 1 && trimmed[0] == trimmed[len(trimmed)-1] && (trimmed[0] == '"' || trimmed[0] == '\'') {
		return trimmed[1 : len(trimmed)-1]
	}
	return trimmed
}
