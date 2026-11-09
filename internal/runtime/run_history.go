package runtime

import (
	"strings"
	"time"

	"quota-activation/internal/activator"
	"quota-activation/internal/state"
)

const maxRunHistory = 5

const (
	runHistoryKindActivation = "activation"
	runHistoryTriggerManual  = "manual"
	runHistoryTriggerAuto    = "auto"
)

// snapshotActivation 仅用于 manual 唤醒记录。
func (r *Runtime) snapshotActivation(trigger string, result activator.Result, err error) {
	at := r.nowUTC()
	entry := buildActivationHistoryEntry(trigger, result, err, at)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prependRunHistoryLocked(entry)
}

func (r *Runtime) currentRunHistory() []RunHistoryEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.runHistory) == 0 {
		return nil
	}
	out := make([]RunHistoryEntry, len(r.runHistory))
	copy(out, r.runHistory)
	return out
}

func (r *Runtime) prependRunHistoryLocked(entry RunHistoryEntry) {
	history := make([]RunHistoryEntry, 0, maxRunHistory)
	history = append(history, entry)
	for i := 0; i < len(r.runHistory) && len(history) < maxRunHistory; i++ {
		history = append(history, r.runHistory[i])
	}
	r.runHistory = history
}

func (r *Runtime) nowUTC() time.Time {
	if r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}

func buildActivationHistoryEntry(trigger string, result activator.Result, err error, at time.Time) RunHistoryEntry {
	if trigger != runHistoryTriggerManual && trigger != runHistoryTriggerAuto {
		trigger = runHistoryTriggerManual
	}
	attempted, succeeded, failed, skipped := 1, 0, 0, 0
	switch result.Status {
	case activator.StatusSuccess:
		if result.Success {
			succeeded = 1
		} else {
			failed = 1
		}
	case activator.StatusSkipped, activator.StatusBusy:
		skipped = 1
	default:
		failed = 1
	}

	providerErr := ""
	if result.LastError != "" {
		providerErr = localizeHistoryMessage(state.Redact(result.LastError))
	} else if err != nil {
		providerErr = localizeHistoryMessage(state.Redact(err.Error()))
	}

	warning := strings.TrimSpace(result.Warning)
	if warning == "" && succeeded == 1 && err != nil {
		warning = localizeHistoryMessage(state.Redact(err.Error()))
		if !isSaveStateWarningText(warning) {
			warning = ""
		}
	}

	providerName := state.Redact(result.Provider)
	message := ""
	if warning != "" && succeeded == 1 {
		message = warning
		if providerErr == "" {
			providerErr = warning
		}
	} else if providerErr != "" {
		message = providerErr
	}

	return RunHistoryEntry{
		At: at, Kind: runHistoryKindActivation, Trigger: trigger,
		Attempted: attempted, Succeeded: succeeded, Failed: failed, Skipped: skipped,
		Providers: []RunHistoryProvider{{
			Name: providerName, Attempted: 1, Succeeded: succeeded, Failed: failed, Skipped: skipped, Error: providerErr,
		}},
		Message:  message,
		WakePath: string(result.WakePath),
	}
}

func localizeHistoryMessage(raw string) string {
	msg := strings.TrimSpace(raw)
	if msg == "" {
		return msg
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "save activation state") ||
		strings.Contains(lower, "save state context") ||
		(strings.Contains(lower, "context canceled") && strings.Contains(lower, "save")) ||
		(strings.Contains(lower, "context cancelled") && strings.Contains(lower, "save")):
		return "唤醒已成功，但状态保存被中断（内部超时/取消），非凭证失效"
	case strings.Contains(lower, "context canceled") || strings.Contains(lower, "context cancelled"):
		return "唤醒已成功，但状态保存被中断（内部超时/取消），非凭证失效"
	case strings.Contains(lower, "activation scheduler did not select target credential") ||
		strings.Contains(msg, "调度器未选中目标凭证"):
		return "调度器未选中目标凭证"
	case strings.Contains(lower, "antigravity activation response missing choices"):
		return "Antigravity 唤醒响应缺少选项"
	case isHostExecutorUnavailableError(msg):
		return hostExecutorUnavailableMessage
	case strings.Contains(lower, "host model execute") && strings.Contains(lower, "unavailable"):
		return hostExecutorUnavailableMessage
	// 旧英文 boost/confirm 错误 → 纯中文（生产路径已改中文，此处兜底历史记录）
	case strings.Contains(lower, "priority boost required but auth document incomplete"):
		return "需要提升优先级，但未能读取完整凭证文件"
	case strings.Contains(lower, "priority boost refused incomplete auth document"):
		return "凭证文档不完整，拒绝写入优先级"
	case strings.Contains(lower, "is not unique highest priority") ||
		strings.Contains(lower, "runtime priority") && strings.Contains(lower, "expected") ||
		strings.Contains(lower, "priority boost confirm failed") ||
		strings.Contains(lower, "runtime auth not found"):
		return "优先级提升后，运行时仍未确认目标为最高优先级"
	case strings.Contains(lower, "priority boost confirm canceled"):
		return "优先级确认已取消"
	case strings.Contains(lower, "priority boost confirm: missing host"):
		return "优先级确认失败：宿主不可用"
	case strings.Contains(lower, "get physical auth file") ||
		strings.Contains(lower, "list auth files for priority boost"):
		return "读取凭证文件失败"
	case strings.Contains(lower, "boost auth priority"):
		return "提升凭证优先级失败"
	case strings.Contains(lower, "auth document is empty") ||
		strings.Contains(lower, "auth document has no fields"):
		return "凭证文档不完整，拒绝写入优先级"
	case strings.Contains(lower, "usage_limit") || strings.Contains(lower, "usage limit"):
		return "Codex唤醒失败：用量额度已耗尽"
	case strings.Contains(lower, "create activation session"):
		return "创建唤醒会话失败"
	case strings.Contains(lower, "available model is required"):
		return "缺少可用模型"
	case strings.Contains(lower, "host model execute failed") ||
		strings.Contains(lower, "host model execute") ||
		strings.Contains(lower, "model execute failed"):
		return "宿主模型执行失败"
	case strings.Contains(lower, "activation error") || strings.Contains(lower, "activator: missing dependency"):
		return "唤醒失败"
	case strings.Contains(lower, "activator: busy") || strings.EqualFold(msg, "busy"):
		return "执行中"
	case strings.Contains(lower, "missing auth_id") || strings.Contains(lower, "invalid request") ||
		strings.HasPrefix(lower, "missing "):
		return "唤醒请求无效：缺少必要字段"
	case strings.Contains(lower, "connection reset by peer") || strings.Contains(lower, "network failure"):
		return "网络连接失败"
	default:
		// 兜底：委托 activator 统一映射，避免历史仍残留英文。
		return activator.LocalizeUserMessage(msg)
	}
}

func isSaveStateWarningText(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(msg, "状态保存") ||
		strings.Contains(lower, "save activation state") ||
		strings.Contains(lower, "save state context") ||
		strings.Contains(lower, "context canceled") ||
		strings.Contains(lower, "context cancelled")
}
