package detector

import (
	"fmt"
	"strings"
	"time"
)

// Evaluate 解析 quota 输入并按 previousCycleKey 判断当前周期是否需要激活。
func Evaluate(input ProbeInput, previousCycleKey string) (Decision, error) {
	authID := strings.TrimSpace(input.AuthID)
	if authID == "" {
		return unknown("缺少 auth id"), fmt.Errorf("auth id: %w", ErrUnknownQuota)
	}
	cycle, err := parseCycle(input)
	if err != nil {
		return unknown("quota 周期未知"), err
	}
	key := buildCycleKey(authID, cycle)
	return Decision{
		Status:   StatusReady,
		Activate: key.String() != strings.TrimSpace(previousCycleKey),
		CycleKey: key,
		Observation: ProbeObservation{
			Provider:   cycle.provider,
			ModelGroup: cycle.modelGroup,
			Window:     cycle.window,
			ResetAt:    cycle.resetAt,
		},
		Reason: "quota 周期可用",
	}, nil
}

func parseCycle(input ProbeInput) (parsedCycle, error) {
	switch input.Provider {
	case ProviderCodex:
		return parseCodex(input.Payload, input.ObservedAt)
	case ProviderAntigravity:
		return parseAntigravity(input.Payload, input.Model)
	case ProviderUnknown:
		return parsedCycle{}, fmt.Errorf("provider unknown: %w", ErrUnknownQuota)
	default:
		return parsedCycle{}, fmt.Errorf("provider unsupported: %w", ErrUnknownQuota)
	}
}

func buildCycleKey(authID string, cycle parsedCycle) CycleKey {
	provider := string(cycle.provider)
	if cycle.modelGroup != ModelGroupNone {
		provider += "/" + string(cycle.modelGroup)
	}
	return CycleKey(authID + ":" + provider + ":" + string(cycle.window) + ":" + cycle.resetAt.UTC().Format(time.RFC3339))
}

func unknown(reason string) Decision {
	return Decision{Status: StatusUnknown, Reason: reason}
}
