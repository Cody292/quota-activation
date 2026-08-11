package detector

import (
	"fmt"
	"strings"
	"time"
)

// Evaluate 解析 quota 输入并按 previousCycleKey 判断当前周期是否需要激活。
// 仅传入 cycle key 时无法识别「同 reset_at + remaining 恢复」；完整语义见 EvaluateWithPrevious。
func Evaluate(input ProbeInput, previousCycleKey string) (Decision, error) {
	return EvaluateWithPrevious(input, PreviousState{CycleKey: previousCycleKey})
}

// EvaluateWithPrevious 在 CycleKey 去重之外，支持 Codex Free remaining 耗尽→恢复的再唤醒。
//
// Activate = true when:
//   (A) computedCycleKey != previous.CycleKey, OR
//   (B) 同 CycleKey 且 current remaining>0，并满足其一：
//       - previous remaining 未知（从未写入 HasRemaining / 旧 success 无 remaining）
//       - previous remaining<=0（明确耗尽后恢复）
//       - current remaining > previous remaining（额度回升）
//
// Activate = false when: 同 cycle 已 success 且 remaining 持续 >0 且未回升（幂等）。
// CycleKey 故意不含 remaining，避免正常消耗破坏幂等。
func EvaluateWithPrevious(input ProbeInput, previous PreviousState) (Decision, error) {
	authID := strings.TrimSpace(input.AuthID)
	if authID == "" {
		return unknown("缺少 auth id"), fmt.Errorf("auth id: %w", ErrUnknownQuota)
	}
	cycle, err := parseCycle(input)
	if err != nil {
		return unknown("quota 周期未知"), err
	}
	key := buildCycleKey(authID, cycle)
	activate, reason := shouldActivate(key, cycle, previous)
	return Decision{
		Status:   StatusReady,
		Activate: activate,
		CycleKey: key,
		Observation: ProbeObservation{
			Provider:     cycle.provider,
			ModelGroup:   cycle.modelGroup,
			Window:       cycle.window,
			ResetAt:      cycle.resetAt,
			Remaining:    cycle.remaining,
			HasRemaining: cycle.hasRemaining,
		},
		Reason: reason,
	}, nil
}

// shouldActivate 实现周期去重与 remaining 恢复合同（不修改 CycleKey 组成）。
func shouldActivate(key CycleKey, cycle parsedCycle, previous PreviousState) (bool, string) {
	prevKey := strings.TrimSpace(previous.CycleKey)
	if key.String() != prevKey {
		return true, "quota 周期可用"
	}
	// 同 CycleKey：current remaining>0 时按 previous remaining 完备性决定是否再唤醒。
	if cycle.hasRemaining && cycle.remaining > 0 {
		if !previous.HasRemaining {
			// 从未成功写入 remaining 耗尽态时，「恢复」条件永远不满足 → 视为可激活。
			return true, "quota remaining 可激活"
		}
		if previous.Remaining <= 0 {
			return true, "quota remaining 恢复"
		}
		if cycle.remaining > previous.Remaining {
			return true, "quota remaining 恢复"
		}
		// 本 cycle 已 success 且 remaining 持续>0（未回升）→ 幂等 skip。
		return false, "quota 周期已处理"
	}
	return false, "quota 周期已处理"
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
