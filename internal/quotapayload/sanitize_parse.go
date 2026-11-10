package quotapayload

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

func strictJSONFloat(raw any) *float64 {
	var f float64
	switch val := raw.(type) {
	case json.Number:
		parsed, err := val.Float64()
		if err != nil {
			return nil
		}
		f = parsed
	case float64:
		f = val
	default:
		return nil
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	if f == 0 {
		f = 0
	}
	return &f
}

func parseStrictFloat64(raw any) *float64 {
	var f float64
	switch val := raw.(type) {
	case json.Number:
		parsed, err := val.Float64()
		if err != nil {
			return nil
		}
		f = parsed
	case float64:
		f = val
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			return nil
		}
		f = parsed
	default:
		return nil
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	if f == 0 {
		f = 0
	}
	return &f
}

func parseStrictInt64(raw any) *int64 {
	var v int64
	switch val := raw.(type) {
	case json.Number:
		parsed, err := val.Int64()
		if err != nil {
			return nil
		}
		v = parsed
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil
		}
		v = int64(val)
	case int64:
		v = val
	case int:
		v = int64(val)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			return nil
		}
		v = parsed
	default:
		return nil
	}
	return &v
}

func formatValidTime(raw any) string {
	parsed, ok := parseAnyTime(raw)
	if !ok {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

func parseAnyTime(raw any) (time.Time, bool) {
	switch value := raw.(type) {
	case string:
		return parseTimeString(value)
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return time.Time{}, false
		}
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
