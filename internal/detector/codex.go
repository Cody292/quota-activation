package detector

import (
	"fmt"
	"time"
)

type codexQuota struct {
	ResetAt            any            `json:"reset_at"`
	ResetAfterSeconds  any            `json:"reset_after_seconds"`
	LimitWindowSeconds any            `json:"limit_window_seconds"`
	Name               any            `json:"name"`
	Type               any            `json:"type"`
	Category           any            `json:"category"`
	Label              any            `json:"label"`
	Bucket             any            `json:"bucket"`
	Scope              any            `json:"scope"`
	RateLimit          codexRateLimit `json:"rate_limit"`
}

type codexRateLimit struct {
	PrimaryWindow   quotaWindow `json:"primary_window"`
	SecondaryWindow quotaWindow `json:"secondary_window"`
}

func parseCodex(payload []byte, observedAt time.Time) (parsedCycle, error) {
	var quota codexQuota
	if err := decodePayload(payload, &quota); err != nil {
		return parsedCycle{}, err
	}
	windows := []quotaWindow{
		{ResetAt: quota.ResetAt, ResetAfterSeconds: quota.ResetAfterSeconds, LimitWindowSeconds: quota.LimitWindowSeconds, Name: quota.Name, Type: quota.Type, Category: quota.Category, Label: quota.Label, Bucket: quota.Bucket, Scope: quota.Scope},
		quota.RateLimit.PrimaryWindow,
		quota.RateLimit.SecondaryWindow,
	}
	// 多窗口时优先 monthly > weekly > 5h，避免 Free 月额度被 5h 窗口抢先匹配。
	var best parsedCycle
	bestRank := -1
	for _, window := range windows {
		resetAt, ok := windowResetAt(observedAt, window)
		if !ok {
			continue
		}
		cycleWindow := classifyWindow(window)
		if cycleWindow == WindowUnknown {
			continue
		}
		rank := windowPreference(cycleWindow)
		if rank > bestRank {
			best = parsedCycle{provider: ProviderCodex, window: cycleWindow, resetAt: resetAt}
			bestRank = rank
		}
	}
	if bestRank < 0 {
		return parsedCycle{}, fmt.Errorf("codex reset_at: %w", ErrUnknownQuota)
	}
	return best, nil
}
