package quotapayload

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"quota-activation/internal/detector"
)

type sanitizedQuotaDocument struct {
	Models             map[string]sanitizedModel `json:"models,omitempty"`
	Buckets            []sanitizedBucket         `json:"buckets,omitempty"`
	Groups             []sanitizedGroup          `json:"groups,omitempty"`
	ResetAt            string                    `json:"reset_at,omitempty"`
	ResetAfterSeconds  *int64                    `json:"reset_after_seconds,omitempty"`
	LimitWindowSeconds *int64                    `json:"limit_window_seconds,omitempty"`
	Name               string                    `json:"name,omitempty"`
	Type               string                    `json:"type,omitempty"`
	Category           string                    `json:"category,omitempty"`
	Label              string                    `json:"label,omitempty"`
	Bucket             string                    `json:"bucket,omitempty"`
	Scope              string                    `json:"scope,omitempty"`
	Remaining          *int64                    `json:"remaining,omitempty"`
	UsedPercent        *float64                  `json:"used_percent,omitempty"`
	RemainingFraction  *float64                  `json:"remainingFraction,omitempty"`
	RateLimit          *sanitizedRateLimit       `json:"rate_limit,omitempty"`
}

type sanitizedRateLimit struct {
	PrimaryWindow   *sanitizedWindow `json:"primary_window,omitempty"`
	SecondaryWindow *sanitizedWindow `json:"secondary_window,omitempty"`
}

type sanitizedWindow struct {
	ResetAt            string   `json:"reset_at,omitempty"`
	ResetAfterSeconds  *int64   `json:"reset_after_seconds,omitempty"`
	LimitWindowSeconds *int64   `json:"limit_window_seconds,omitempty"`
	Name               string   `json:"name,omitempty"`
	Type               string   `json:"type,omitempty"`
	Category           string   `json:"category,omitempty"`
	Label              string   `json:"label,omitempty"`
	Bucket             string   `json:"bucket,omitempty"`
	Scope              string   `json:"scope,omitempty"`
	ResetTime          string   `json:"resetTime,omitempty"`
	Remaining          *int64   `json:"remaining,omitempty"`
	UsedPercent        *float64 `json:"used_percent,omitempty"`
	RemainingFraction  *float64 `json:"remainingFraction,omitempty"`
}

// Sanitize 仅保留配额决策与展示所需的白名单字段，剥离任意嵌套及敏感字段。
func Sanitize(raw []byte) (json.RawMessage, bool) {
	return sanitizeDocument(raw, true, true)
}

// SanitizeForProvider 按指定提供商白名单清洗 quota_payload，隔离无关域与嵌套敏感数据。
func SanitizeForProvider(provider detector.Provider, raw []byte) (json.RawMessage, bool) {
	switch provider {
	case detector.ProviderCodex:
		return sanitizeDocument(raw, false, true)
	case detector.ProviderAntigravity:
		return sanitizeDocument(raw, true, false)
	case detector.ProviderUnknown:
		return nil, false
	default:
		return nil, false
	}
}

func sanitizeDocument(raw []byte, allowAntigravity bool, allowCodex bool) (json.RawMessage, bool) {
	root, ok := decodeJSONObject(raw)
	if !ok {
		return nil, false
	}
	var doc sanitizedQuotaDocument
	if allowAntigravity {
		doc.Models = sanitizeModels(root["models"])
		doc.Buckets = sanitizeBuckets(root["buckets"])
		doc.Groups = sanitizeGroups(root["groups"])
	}
	if allowCodex {
		topWindow := sanitizeWindowMap(root)
		doc.ResetAt = topWindow.ResetAt
		doc.ResetAfterSeconds = topWindow.ResetAfterSeconds
		doc.LimitWindowSeconds = topWindow.LimitWindowSeconds
		doc.Name = topWindow.Name
		doc.Type = topWindow.Type
		doc.Category = topWindow.Category
		doc.Label = topWindow.Label
		doc.Bucket = topWindow.Bucket
		doc.Scope = topWindow.Scope
		doc.Remaining = topWindow.Remaining
		doc.UsedPercent = topWindow.UsedPercent
		doc.RemainingFraction = topWindow.RemainingFraction
		doc.RateLimit = sanitizeRateLimit(root["rate_limit"])
	}
	if !doc.hasContent() {
		return nil, false
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, false
	}
	return json.RawMessage(encoded), true
}

func (d sanitizedQuotaDocument) hasContent() bool {
	return len(d.Models) > 0 || len(d.Buckets) > 0 || len(d.Groups) > 0 ||
		d.ResetAt != "" || d.ResetAfterSeconds != nil || d.LimitWindowSeconds != nil ||
		d.Name != "" || d.Type != "" || d.Category != "" || d.Label != "" ||
		d.Bucket != "" || d.Scope != "" || d.Remaining != nil ||
		d.UsedPercent != nil || d.RemainingFraction != nil || d.RateLimit != nil
}

func decodeJSONObject(raw []byte) (map[string]any, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil || root == nil {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return root, true
}

func sanitizeRateLimit(raw any) *sanitizedRateLimit {
	rateMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	var rate sanitizedRateLimit
	if primaryMap, ok := rateMap["primary_window"].(map[string]any); ok {
		if w := sanitizeWindowMap(primaryMap); w.hasContent() {
			rate.PrimaryWindow = &w
		}
	}
	if secondaryMap, ok := rateMap["secondary_window"].(map[string]any); ok {
		if w := sanitizeWindowMap(secondaryMap); w.hasContent() {
			rate.SecondaryWindow = &w
		}
	}
	if rate.PrimaryWindow == nil && rate.SecondaryWindow == nil {
		return nil
	}
	return &rate
}

func sanitizeWindowMap(m map[string]any) sanitizedWindow {
	return sanitizedWindow{
		ResetAt:            formatValidTime(m["reset_at"]),
		ResetAfterSeconds:  parseStrictInt64(m["reset_after_seconds"]),
		LimitWindowSeconds: parseStrictInt64(m["limit_window_seconds"]),
		Name:               canonicalWindowText(m["name"]),
		Type:               canonicalWindowText(m["type"]),
		Category:           canonicalWindowText(m["category"]),
		Label:              canonicalWindowText(m["label"]),
		Bucket:             canonicalWindowText(m["bucket"]),
		Scope:              canonicalWindowText(m["scope"]),
		ResetTime:          formatValidTime(m["resetTime"]),
		Remaining:          parseStrictInt64(m["remaining"]),
		UsedPercent:        parseStrictFloat64(m["used_percent"]),
		RemainingFraction:  strictJSONFloat(m["remainingFraction"]),
	}
}

func (w sanitizedWindow) hasContent() bool {
	return w.ResetAt != "" || w.ResetAfterSeconds != nil || w.LimitWindowSeconds != nil ||
		w.Name != "" || w.Type != "" || w.Category != "" || w.Label != "" ||
		w.Bucket != "" || w.Scope != "" || w.ResetTime != "" || w.Remaining != nil ||
		w.UsedPercent != nil || w.RemainingFraction != nil
}

func canonicalWindowText(raw any) string {
	text, ok := raw.(string)
	if !ok {
		return ""
	}
	normalized := strings.ToLower(strings.TrimSpace(text))
	normalized = strings.ReplaceAll(normalized, "_", " ")
	normalized = strings.ReplaceAll(normalized, "-", " ")
	switch {
	case strings.Contains(normalized, "monthly") || strings.Contains(normalized, "month") || strings.Contains(normalized, "30d") || strings.Contains(normalized, "30 day"):
		return string(detector.WindowMonthly)
	case strings.Contains(normalized, "weekly") || strings.Contains(normalized, "week") || strings.Contains(normalized, "7d"):
		return string(detector.WindowWeekly)
	case strings.Contains(normalized, "5h") || strings.Contains(normalized, "5 hour") || strings.Contains(normalized, "5 hr") || strings.Contains(normalized, "3h"):
		return string(detector.WindowFiveHour)
	default:
		return ""
	}
}
