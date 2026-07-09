package detector

import "strings"

func classifyWindow(window quotaWindow) Window {
	if seconds, ok := toInt64(window.LimitWindowSeconds); ok {
		switch seconds {
		case 3 * 60 * 60, 5 * 60 * 60:
			return WindowFiveHour
		case 7 * 24 * 60 * 60:
			return WindowWeekly
		}
	}
	for _, text := range windowTexts(window) {
		if strings.Contains(text, "weekly") || strings.Contains(text, "week") || strings.Contains(text, "7d") {
			return WindowWeekly
		}
		if strings.Contains(text, "5h") || strings.Contains(text, "5 hour") || strings.Contains(text, "5 hr") || strings.Contains(text, "3h") {
			return WindowFiveHour
		}
	}
	return WindowUnknown
}

func windowTexts(window quotaWindow) []string {
	fields := []any{window.Name, window.Type, window.Category, window.Label, window.Bucket, window.Scope}
	texts := make([]string, 0, len(fields))
	for _, field := range fields {
		text, ok := field.(string)
		if !ok {
			continue
		}
		normalized := strings.ToLower(strings.TrimSpace(text))
		normalized = strings.ReplaceAll(normalized, "_", " ")
		normalized = strings.ReplaceAll(normalized, "-", " ")
		if normalized != "" {
			texts = append(texts, normalized)
		}
	}
	return texts
}
