package runtime

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const runHistoryKindScan = "scan"

type scanSummaryInput struct {
	Attempted   int
	Succeeded   int
	Failed      int
	Skipped     int
	SkipReasons map[string]int
	FailReasons map[string]int
	Providers   []RunHistoryProvider
	Warning     string
}

type scanSummaryAccumulator struct {
	mu               sync.Mutex
	attempted        int
	succeeded        int
	failed           int
	skipped          int
	skipReasons      map[string]int
	failReasons      map[string]int
	byProvider       map[string]*RunHistoryProvider
	providerFailures map[string]map[string]int
	warning          string
}

func newScanSummaryAccumulator() *scanSummaryAccumulator {
	return &scanSummaryAccumulator{
		skipReasons:      make(map[string]int),
		failReasons:      make(map[string]int),
		byProvider:       make(map[string]*RunHistoryProvider),
		providerFailures: make(map[string]map[string]int),
	}
}

func (a *scanSummaryAccumulator) add(provider string, detail autoScanDetail) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.attempted++
	name := strings.TrimSpace(provider)
	if name == "" {
		name = "unknown"
	}
	p := a.byProvider[name]
	if p == nil {
		p = &RunHistoryProvider{Name: name}
		a.byProvider[name] = p
	}
	p.Attempted++
	switch detail.outcome {
	case autoScanOutcomeSucceeded:
		a.succeeded++
		p.Succeeded++
	case autoScanOutcomeFailed:
		a.failed++
		p.Failed++
		reason := strings.TrimSpace(detail.errMessage)
		if reason == "" {
			reason = strings.TrimSpace(detail.reason)
		}
		if reason == "" {
			reason = "唤醒失败"
		}
		reason = localizeHistoryMessage(reason)
		a.failReasons[reason]++
		providerReasons := a.providerFailures[name]
		if providerReasons == nil {
			providerReasons = make(map[string]int)
			a.providerFailures[name] = providerReasons
		}
		providerReasons[reason]++
	case autoScanOutcomeSkipped:
		a.skipped++
		p.Skipped++
		reason := strings.TrimSpace(detail.reason)
		if reason != "" {
			a.skipReasons[reason]++
		}
	}
	if a.warning == "" && isSaveStateWarningText(detail.warning) {
		a.warning = strings.TrimSpace(detail.warning)
	}
}

func (a *scanSummaryAccumulator) toInput() scanSummaryInput {
	a.mu.Lock()
	defer a.mu.Unlock()
	providers := make([]RunHistoryProvider, 0, len(a.byProvider))
	for _, p := range a.byProvider {
		if reasons := a.providerFailures[p.Name]; len(reasons) > 0 {
			formatted := make([]string, 0, len(reasons))
			for reason, count := range reasons {
				formatted = append(formatted, fmt.Sprintf("%s ×%d", reason, count))
			}
			sort.Strings(formatted)
			p.Error = strings.Join(formatted, "，")
		}
		providers = append(providers, *p)
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})
	return scanSummaryInput{
		Attempted: a.attempted, Succeeded: a.succeeded, Failed: a.failed, Skipped: a.skipped,
		SkipReasons: a.skipReasons, FailReasons: a.failReasons, Providers: providers,
		Warning: a.warning,
	}
}

func (r *Runtime) snapshotScanSummary(in scanSummaryInput) {
	at := r.nowUTC()
	entry := buildScanSummaryHistoryEntry(in, at)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prependRunHistoryLocked(entry)
}

func buildScanSummaryHistoryEntry(in scanSummaryInput, at time.Time) RunHistoryEntry {
	return RunHistoryEntry{
		At: at, Kind: runHistoryKindScan, Trigger: runHistoryTriggerAuto,
		Attempted: in.Attempted, Succeeded: in.Succeeded, Failed: in.Failed, Skipped: in.Skipped,
		Providers: in.Providers, Message: formatScanSummaryMessage(in),
	}
}

func formatScanSummaryMessage(in scanSummaryInput) string {
	warning := strings.TrimSpace(in.Warning)
	if warning == "" || !isSaveStateWarningText(warning) {
		return ""
	}
	return warning
}

func formatTopReasons(label string, reasons map[string]int, maxN int) string {
	if len(reasons) == 0 || maxN <= 0 {
		return ""
	}
	type reasonCount struct {
		reason string
		count  int
	}
	items := make([]reasonCount, 0, len(reasons))
	for reason, count := range reasons {
		reason = strings.TrimSpace(reason)
		if reason == "" || count <= 0 {
			continue
		}
		items = append(items, reasonCount{reason: localizeSkipReason(reason), count: count})
	}
	if len(items) == 0 {
		return ""
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].reason < items[j].reason
	})
	if len(items) > maxN {
		items = items[:maxN]
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s ×%d", item.reason, item.count))
	}
	return label + "：" + strings.Join(parts, "，")
}

func localizeSkipReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	switch {
	case trimmed == "":
		return trimmed
	case strings.Contains(trimmed, "额度周期已处理") ||
		strings.Contains(trimmed, "本周期已唤醒") ||
		strings.Contains(trimmed, "quota 周期已处理") ||
		strings.EqualFold(trimmed, "quota cycle already processed"):
		return "额度周期已处理"
	case strings.Contains(trimmed, "额度已恢复") ||
		strings.Contains(trimmed, "quota remaining 恢复"):
		return "额度已恢复"
	case strings.Contains(trimmed, "额度周期可用") ||
		strings.Contains(trimmed, "quota 周期可用"):
		return "额度周期可用"
	case strings.Contains(trimmed, "remaining") && (strings.Contains(trimmed, "可激活") || strings.Contains(strings.ToLower(trimmed), "activat")):
		return "额度可激活"
	case strings.EqualFold(trimmed, "busy") || trimmed == "执行中":
		return "执行中"
	case strings.Contains(trimmed, "调度未选中目标凭证") || strings.Contains(trimmed, "调度器未选中目标凭证"):
		return "调度未选中目标凭证（已冷却）"
	case isHostExecutorUnavailableError(trimmed) || strings.Contains(trimmed, hostExecutorUnavailableCooldownReason):
		return hostExecutorUnavailableCooldownReason
	default:
		return localizeHistoryMessage(trimmed)
	}
}
