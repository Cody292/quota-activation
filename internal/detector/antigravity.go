package detector

import (
	"fmt"
	"strings"
)

type antigravityQuota struct {
	Models  map[string]antigravityModel `json:"models"`
	Groups  []antigravityGroup          `json:"groups"`
	Buckets []antigravityBucket         `json:"buckets"`
}

type antigravityModel struct {
	ModelProvider string          `json:"modelProvider"`
	QuotaInfo     antigravityInfo `json:"quotaInfo"`
}

type antigravityInfo struct {
	ResetTime any           `json:"resetTime"`
	Windows   []quotaWindow `json:"windows"`
}

type antigravityGroup struct {
	DisplayName string              `json:"displayName"`
	Description string              `json:"description"`
	Buckets     []antigravityBucket `json:"buckets"`
}

type antigravityBucket struct {
	ModelID   string `json:"modelId"`
	Window    string `json:"window"`
	ResetTime any    `json:"resetTime"`
}

func parseAntigravity(payload []byte, model string) (parsedCycle, error) {
	modelGroup, ok := inferModelGroup(model)
	if !ok {
		return parsedCycle{}, fmt.Errorf("antigravity model group: %w", ErrUnknownQuota)
	}
	var quota antigravityQuota
	if err := decodePayload(payload, &quota); err != nil {
		return parsedCycle{}, err
	}
	if cycle, ok := parseAntigravityModels(quota.Models, modelGroup); ok {
		return cycle, nil
	}
	if cycle, ok := parseAntigravityBuckets(quota.Buckets, modelGroup); ok {
		return cycle, nil
	}
	if cycle, ok := parseAntigravityGroups(quota.Groups, modelGroup); ok {
		return cycle, nil
	}
	return parsedCycle{}, fmt.Errorf("antigravity reset_at: %w", ErrUnknownQuota)
}

func parseAntigravityModels(models map[string]antigravityModel, group ModelGroup) (parsedCycle, bool) {
	for modelID, item := range models {
		if !belongsToModelGroup(modelID+" "+item.ModelProvider, group) {
			continue
		}
		windows := item.QuotaInfo.Windows
		if len(windows) == 0 {
			windows = []quotaWindow{{ResetTime: item.QuotaInfo.ResetTime}}
		}
		if cycle, ok := firstAntigravityWindow(windows, group); ok {
			return cycle, true
		}
	}
	return parsedCycle{}, false
}

func parseAntigravityBuckets(buckets []antigravityBucket, group ModelGroup) (parsedCycle, bool) {
	for _, bucket := range buckets {
		if !belongsToModelGroup(bucket.ModelID, group) {
			continue
		}
		window := quotaWindow{ResetTime: bucket.ResetTime, Name: bucket.Window}
		if cycle, ok := antigravityWindow(window, group); ok {
			return cycle, true
		}
	}
	return parsedCycle{}, false
}

func parseAntigravityGroups(groups []antigravityGroup, modelGroup ModelGroup) (parsedCycle, bool) {
	for _, group := range groups {
		if !belongsToModelGroup(group.DisplayName+" "+group.Description, modelGroup) {
			continue
		}
		if cycle, ok := parseAntigravityBuckets(group.Buckets, modelGroup); ok {
			return cycle, true
		}
	}
	return parsedCycle{}, false
}

func firstAntigravityWindow(windows []quotaWindow, group ModelGroup) (parsedCycle, bool) {
	for _, window := range windows {
		cycle, ok := antigravityWindow(window, group)
		if ok {
			return cycle, true
		}
	}
	return parsedCycle{}, false
}

func antigravityWindow(window quotaWindow, group ModelGroup) (parsedCycle, bool) {
	resetAt, ok := parseAnyTime(window.ResetTime)
	if !ok {
		return parsedCycle{}, false
	}
	cycleWindow := classifyWindow(window)
	if cycleWindow == WindowUnknown {
		return parsedCycle{}, false
	}
	return parsedCycle{provider: ProviderAntigravity, modelGroup: group, window: cycleWindow, resetAt: resetAt}, true
}

func inferModelGroup(model string) (ModelGroup, bool) {
	text := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(text, "claude") || strings.Contains(text, "gpt") || strings.Contains(text, "openai") {
		return ModelGroupClaudeGPT, true
	}
	if strings.Contains(text, "gemini") {
		return ModelGroupGemini, true
	}
	return ModelGroupNone, false
}

func belongsToModelGroup(text string, group ModelGroup) bool {
	groupText := strings.ToLower(strings.TrimSpace(text))
	switch group {
	case ModelGroupClaudeGPT:
		return strings.Contains(groupText, "claude") || strings.Contains(groupText, "gpt") || strings.Contains(groupText, "openai")
	case ModelGroupGemini:
		return strings.Contains(groupText, "gemini") && !strings.Contains(groupText, "claude") && !strings.Contains(groupText, "gpt")
	case ModelGroupNone:
		return false
	default:
		return false
	}
}
