package detector

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

func windowResetAt(observedAt time.Time, window quotaWindow) (time.Time, bool) {
	if resetAt, ok := parseAnyTime(window.ResetAt); ok {
		return resetAt, true
	}
	seconds, ok := toInt64(window.ResetAfterSeconds)
	if !ok || seconds <= 0 {
		return time.Time{}, false
	}
	return observedAt.UTC().Add(time.Duration(seconds) * time.Second), true
}

func parseAnyTime(raw any) (time.Time, bool) {
	switch value := raw.(type) {
	case nil:
		return time.Time{}, false
	case string:
		return parseTimeString(value)
	case float64:
		return parseUnix(int64(value))
	case json.Number:
		integer, err := value.Int64()
		if err != nil {
			return time.Time{}, false
		}
		return parseUnix(integer)
	default:
		return time.Time{}, false
	}
}

func parseTimeString(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, false
	}
	if integer, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return parseUnix(integer)
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		parsed, err := time.Parse(layout, trimmed)
		if err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func parseUnix(value int64) (time.Time, bool) {
	if value <= 0 {
		return time.Time{}, false
	}
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC(), true
	}
	return time.Unix(value, 0).UTC(), true
}

func toInt64(raw any) (int64, bool) {
	switch value := raw.(type) {
	case float64:
		return int64(value), true
	case json.Number:
		integer, err := value.Int64()
		return integer, err == nil
	case string:
		integer, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return integer, err == nil
	default:
		return 0, false
	}
}

func toFloat64(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case json.Number:
		f, err := value.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return f, err == nil
	default:
		return 0, false
	}
}
