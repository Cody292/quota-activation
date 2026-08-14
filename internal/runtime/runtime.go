package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"quota-activation/internal/activator"
	"quota-activation/internal/config"
	"quota-activation/internal/host"
	"quota-activation/internal/management"
	"quota-activation/internal/scheduler"
	"quota-activation/internal/session"
	"quota-activation/internal/state"
)

// Runtime 持有 CPA 插件生命周期、配置和调度/管理依赖。
type Runtime struct {
	mu   sync.Mutex
	host host.Client
	now  func() time.Time
	// sleep 可注入：sleep(ctx, d) 在 d 内阻塞，ctx 取消返回 false。
	sleep            func(context.Context, time.Duration) bool
	config           config.Config
	sessions         *session.Manager
	store            *state.Store
	activator        *activator.Activator
	management       *management.Handler
	picker           scheduler.Picker
	autoCancel       context.CancelFunc
	autoScan         func(context.Context)
	autoScanInterval time.Duration
	// lastAutoScanAt 最近一次实际执行 auto scan 的时间（mu 保护）；门闩强制遵守 ScanInterval。
	lastAutoScanAt time.Time
	// autoScanRunning 防重入：上一轮未完成时拒绝新一轮 claim。
	autoScanRunning bool
	// startupDelay nil 表示默认 autoScanStartupDelay；非 nil 为测试注入（含 0）。
	startupDelay *time.Duration
	// schedulerMissUntil 记录可冷却失败（调度未选中 / 宿主执行器不可用）的 per-auth 截止时间。
	schedulerMissUntil map[string]time.Time
	// activateCooldownReasonByAuth 冷却 skip 原因（与 schedulerMissUntil 同步）。
	activateCooldownReasonByAuth map[string]string
	runHistory                   []RunHistoryEntry
	shutdown                     bool
}

// New 创建未注册 runtime；首次 register/reconfigure 成功后才启用依赖。
func New(options Options) *Runtime {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	sleep := options.Sleep
	if sleep == nil {
		sleep = sleepWithContext
	}
	runtime := &Runtime{host: options.Host, now: now, sleep: sleep, config: config.Default(), startupDelay: options.StartupDelay}
	runtime.autoScan = runtime.runAutoScanLoop
	return runtime
}

// sleepWithContext 在 d 内阻塞；ctx 取消返回 false。
func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Handle 根据 CPA 方法名处理 JSON 请求并返回 JSON 信封字节。
func (r *Runtime) Handle(ctx context.Context, method string, request []byte) []byte {
	switch method {
	case "plugin.register":
		parsed, err := decodeRegisterRequest(request)
		if err != nil {
			return failure(err)
		}
		result, err := r.Register(ctx, parsed)
		return envelopeResult(result, err)
	case "plugin.reconfigure":
		parsed, err := decodeReconfigureRequest(request)
		if err != nil {
			return failure(err)
		}
		result, err := r.Reconfigure(ctx, parsed)
		return envelopeResult(result, err)
	case "plugin.shutdown":
		return envelopeStatus(r.Shutdown(ctx))
	case "management.register":
		return r.registerManagement()
	case "management.handle":
		return r.handleManagement(ctx, request)
	case "scheduler.pick":
		return r.pickSchedule(request)
	default:
		return failure(fmt.Errorf("%w: method %q", ErrInvalidRequest, method))
	}
}

// Register 解析首次配置并返回 management_api + scheduler 能力声明。
func (r *Runtime) Register(ctx context.Context, request RegisterRequest) (RegisterResult, error) {
	cfg, err := config.Parse([]byte(request.ConfigYAML))
	if err != nil {
		return RegisterResult{}, fmt.Errorf("load register config: %w", err)
	}
	if err := r.replaceConfig(ctx, cfg); err != nil {
		return RegisterResult{}, err
	}
	return registrationResult(), nil
}

// Reconfigure 验证并替换运行时配置。
func (r *Runtime) Reconfigure(ctx context.Context, request ReconfigureRequest) (RegisterResult, error) {
	cfg, err := config.Parse([]byte(request.ConfigYAML))
	if err != nil {
		return RegisterResult{}, fmt.Errorf("load reconfigure config: %w", err)
	}
	if err := r.replaceConfig(ctx, cfg); err != nil {
		return RegisterResult{}, err
	}
	return registrationResult(), nil
}

// Shutdown 将 runtime 标记为关闭，并等待在途 auto scan 结束（避免 TempDir/状态写盘竞态）。
func (r *Runtime) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("runtime shutdown context: %w", err)
	}
	r.mu.Lock()
	r.stopAutoScannerLocked()
	r.shutdown = true
	r.mu.Unlock()
	// 等在途 scan/Activate/SaveAtomic 结束，防止测试 TempDir 清理撞上 .tmp 写盘。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		running := r.autoScanRunning
		r.mu.Unlock()
		if !running {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("runtime shutdown context: %w", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

func (r *Runtime) replaceConfig(ctx context.Context, cfg config.Config) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("runtime configure context: %w", err)
	}
	store, err := state.Load(ctx, cfg.StatePath)
	if err != nil {
		return err
	}
	sessions := session.NewManager(session.Options{Now: r.now})
	activation := activator.New(activator.Options{Host: r.host, Sessions: sessions, State: store, Config: cfg, Now: r.now, Sleep: r.sleep})
	// 闭包捕获 Runtime：手动激活写 run_history，diagnostics 读 run_history。
	manager := management.NewHandler(management.Options{
		Activator: activation,
		Host:      r.host,
		Store:     store,
		Config:    cfg,
		Now:       r.now,
		OnActivation: func(result activator.Result, err error) {
			r.snapshotActivation(runHistoryTriggerManual, result, err)
			if err == nil && result.Success && result.Status == activator.StatusSuccess {
				r.clearSchedulerMissCooldown(result.AuthID)
			}
		},
		RunHistory: func() any {
			return r.currentRunHistory()
		},
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shutdown {
		return ErrShutdown
	}
	r.config = cfg
	r.sessions = sessions
	r.store = store
	r.activator = activation
	r.management = manager
	r.picker = scheduler.NewPicker(sessions)
	// reconfigure 防抖：AutoActivate 仍 true 且 interval 未变时不 stop+start，避免 immediate 首扫狂刷。
	if !cfg.AutoActivate {
		r.stopAutoScannerLocked()
	} else {
		r.startAutoScannerLocked(cfg.ScanInterval)
	}
	return nil
}

func (r *Runtime) startAutoScannerLocked(interval time.Duration) {
	if interval <= 0 {
		interval = config.Default().ScanInterval
	}
	// 已在跑且 interval 相同 → no-op（宿主频繁 reconfigure 的主路径）。
	if r.autoCancel != nil && r.autoScanInterval == interval {
		return
	}
	r.stopAutoScannerLocked()
	ctx, cancel := context.WithCancel(context.Background())
	r.autoCancel = cancel
	r.autoScanInterval = interval
	go r.autoScan(ctx)
}

func (r *Runtime) stopAutoScannerLocked() {
	if r.autoCancel == nil {
		return
	}
	r.autoCancel()
	r.autoCancel = nil
	r.autoScanInterval = 0
}

func registrationResult() RegisterResult {
	return RegisterResult{
		SchemaVersion: 1,
		Metadata: Metadata{
			Name:             "quota-activation",
			Version:          "0.0.5",
			Author:           "Cody292",
			GitHubRepository: "https://github.com/Cody292/quota-activation",
			Description:      "Quota reset activation management API and scheduler helper for Codex and Antigravity.",
			ConfigFields:     configFields(),
		},
		Capabilities: map[string]bool{"management_api": true, "scheduler": true},
	}
}

func configFields() []ConfigField {
	defaults := config.Default()
	return []ConfigField{
		{Name: "activation_models.codex.models", Type: "string", Description: localizedDescription("Codex 自动唤醒使用的模型名称。", "Model name used for Codex automatic activation."), DefaultValue: defaults.ActivationModels.Codex.Models},
		{Name: "activation_models.antigravity.models_group", Type: "string", Description: localizedDescription("Antigravity 自动唤醒使用的模型组。", "Model group used for Antigravity automatic activation."), EnumValues: []string{"gemini", "claude_gpt"}, DefaultValue: defaults.ActivationModels.Antigravity.ModelsGroup},
		{Name: "activation_models.antigravity.models", Type: "string", Description: localizedDescription("当前 Antigravity 模型组自动唤醒使用的模型名称。", "Model name used for the selected Antigravity model group."), DefaultValue: defaults.ActivationModels.Antigravity.Models},
		{Name: "auto_activate", Type: "boolean", Description: localizedDescription("启用调度器自动配额唤醒；默认 false，因此手动唤醒仍是默认方式。未启用时执行记录不会出现 auto scan 摘要。", "Enable scheduler-driven automatic quota activation; default is false, so manual activation remains the default. When disabled, no auto scan summary appears in run history."), DefaultValue: defaults.AutoActivate},
		{Name: "enable_before_activation", Type: "boolean", Description: localizedDescription("默认 true：达到唤醒条件后自动启用已禁用凭证并保持启用；显式 false 时候选阶段丢弃 disabled。", "Default true: automatically enable disabled credentials that meet activation conditions and keep them enabled; explicit false drops them at candidate stage."), DefaultValue: defaults.EnableBeforeActivation},
		{Name: "scan_interval", Type: "string", Description: localizedDescription("自动扫描间隔（单位：分钟）。填写纯数字即可，无需带单位；默认 30。", "Automatic scan interval in minutes. Enter a plain number without a unit; default is 30."), DefaultValue: formatDurationMinutes(defaults.ScanInterval)},
		{Name: "activation_request_timeout", Type: "string", Description: localizedDescription("唤醒模型请求超时（单位：秒）。填写纯数字即可，无需带单位；默认 60。", "Activation model request timeout in seconds. Enter a plain number without a unit; default is 60."), DefaultValue: formatDurationSeconds(defaults.ActivationRequestTimeout)},
		{Name: "max_concurrency", Type: "integer", Description: localizedDescription("自动扫描与激活的并发上限（worker 池大小）；默认 1。", "Concurrency limit for automatic scan activation (worker pool size); default 1."), DefaultValue: defaults.MaxConcurrency},
		{Name: "activation_prompt", Type: "string", Description: localizedDescription("通过 host.model.execute 发送的配额唤醒提示词。", "Prompt sent through host.model.execute for quota activation."), DefaultValue: defaults.ActivationPrompt},
		{Name: "activation_transport", Type: "string", Description: localizedDescription("唤醒传输方式：direct_http（默认，经 host.http.do）或 scheduler_boost。", "Activation transport: direct_http (default, via host.http.do) or scheduler_boost."), EnumValues: []string{string(config.ActivationTransportDirectHTTP), string(config.ActivationTransportSchedulerBoost)}, DefaultValue: string(defaults.ActivationTransport)},
		{Name: "scheduler_boost_fallback", Type: "boolean", Description: localizedDescription("direct_http 遇传输/宿主类失败时是否回退 scheduler_boost；默认 true。业务失败不回退。", "When direct_http hits transport/host failures, fall back to scheduler_boost; default true. Business failures never fall back."), DefaultValue: defaults.SchedulerBoostFallback},
	}
}
func localizedDescription(chinese string, english string) string {
	return chinese + "\n" + english
}

func formatDurationMinutes(value time.Duration) string {
	if value <= 0 {
		return "30"
	}
	minutes := int(value / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("%d", minutes)
}

func formatDurationSeconds(value time.Duration) string {
	if value <= 0 {
		return "60"
	}
	seconds := int(value / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf("%d", seconds)
}
