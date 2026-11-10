package quotapayload

import "sort"

type sanitizedModel struct {
	ModelProvider string              `json:"modelProvider,omitempty"`
	QuotaInfo     *sanitizedQuotaInfo `json:"quotaInfo,omitempty"`
}

type sanitizedQuotaInfo struct {
	ResetTime         string            `json:"resetTime,omitempty"`
	RemainingFraction *float64          `json:"remainingFraction,omitempty"`
	Windows           []sanitizedWindow `json:"windows,omitempty"`
}

type sanitizedBucket struct {
	ModelID           string   `json:"modelId,omitempty"`
	Window            string   `json:"window,omitempty"`
	ResetTime         string   `json:"resetTime,omitempty"`
	RemainingFraction *float64 `json:"remainingFraction,omitempty"`
}

type sanitizedGroup struct {
	DisplayName string            `json:"displayName,omitempty"`
	Buckets     []sanitizedBucket `json:"buckets,omitempty"`
}

func sanitizeModels(raw any) map[string]sanitizedModel {
	rawMap, ok := raw.(map[string]any)
	if !ok || len(rawMap) == 0 {
		return nil
	}
	keys := make([]string, 0, len(rawMap))
	for k := range rawMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	models := make(map[string]sanitizedModel)
	// 先保留合法无污染的模型键，避免脏键回退映射时覆盖干净模型。
	for _, passClean := range []bool{true, false} {
		for _, rawKey := range keys {
			entryMap, ok := rawMap[rawKey].(map[string]any)
			if !ok {
				continue
			}
			provider := sanitizeModelProvider(entryMap["modelProvider"])
			modelKey, exactClean, ok := sanitizeModelKey(rawKey, provider)
			if !ok || exactClean != passClean {
				continue
			}
			info := sanitizeQuotaInfo(entryMap["quotaInfo"])
			if info == nil {
				continue
			}
			if _, exists := models[modelKey]; exists {
				continue
			}
			models[modelKey] = sanitizedModel{ModelProvider: provider, QuotaInfo: info}
		}
	}
	if len(models) == 0 {
		return nil
	}
	return models
}

func sanitizeQuotaInfo(raw any) *sanitizedQuotaInfo {
	infoMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	info := &sanitizedQuotaInfo{
		ResetTime:         formatValidTime(infoMap["resetTime"]),
		RemainingFraction: strictJSONFloat(infoMap["remainingFraction"]),
		Windows:           sanitizeWindows(infoMap["windows"]),
	}
	if info.ResetTime == "" && info.RemainingFraction == nil && len(info.Windows) == 0 {
		return nil
	}
	return info
}

func sanitizeWindows(raw any) []sanitizedWindow {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]sanitizedWindow, 0, len(items))
	for _, item := range items {
		windowMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		window := sanitizeWindowMap(windowMap)
		if window.hasContent() {
			out = append(out, window)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeBuckets(raw any) []sanitizedBucket {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]sanitizedBucket, 0, len(items))
	for _, item := range items {
		bucketMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rawModelID, _ := bucketMap["modelId"].(string)
		modelID, _, ok := sanitizeModelKey(rawModelID, "")
		if !ok {
			continue
		}
		bucket := sanitizedBucket{
			ModelID:           modelID,
			Window:            canonicalWindowText(bucketMap["window"]),
			ResetTime:         formatValidTime(bucketMap["resetTime"]),
			RemainingFraction: strictJSONFloat(bucketMap["remainingFraction"]),
		}
		if bucket.Window == "" && bucket.ResetTime == "" && bucket.RemainingFraction == nil {
			continue
		}
		out = append(out, bucket)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeGroups(raw any) []sanitizedGroup {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]sanitizedGroup, 0, len(items))
	for _, item := range items {
		groupMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		displayName := sanitizeGroupDisplayName(groupMap["displayName"], groupMap["description"])
		if displayName == "" {
			continue
		}
		buckets := sanitizeBuckets(groupMap["buckets"])
		if len(buckets) == 0 {
			continue
		}
		out = append(out, sanitizedGroup{DisplayName: displayName, Buckets: buckets})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
