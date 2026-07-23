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
	mu               sync.Mutex
	host             host.Client
	now              func() time.Time
	config           config.Config
	sessions         *session.Manager
	store            *state.Store
	activator        *activator.Activator
	management       *management.Handler
	picker           scheduler.Picker
	autoCancel       context.CancelFunc
	autoScan         func(context.Context)
	autoScanInterval time.Duration
	shutdown         bool
}

// New 创建未注册 runtime；首次 register/reconfigure 成功后才启用依赖。
func New(options Options) *Runtime {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	runtime := &Runtime{host: options.Host, now: now, config: config.Default()}
	runtime.autoScan = runtime.runAutoScanLoop
	return runtime
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

// Shutdown 将 runtime 标记为关闭。
func (r *Runtime) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("runtime shutdown context: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopAutoScannerLocked()
	r.shutdown = true
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
	activation := activator.New(activator.Options{Host: r.host, Sessions: sessions, State: store, Config: cfg, Now: r.now})
	manager := management.NewHandler(management.Options{Activator: activation, Host: r.host, Config: cfg, Now: r.now})
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shutdown {
		return ErrShutdown
	}
	r.stopAutoScannerLocked()
	r.config = cfg
	r.sessions = sessions
	r.store = store
	r.activator = activation
	r.management = manager
	r.picker = scheduler.NewPicker(sessions)
	if cfg.AutoActivate {
		r.startAutoScannerLocked(cfg.ScanInterval)
	}
	return nil
}

func (r *Runtime) startAutoScannerLocked(interval time.Duration) {
	if interval <= 0 {
		interval = config.Default().ScanInterval
	}
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
			Version:          "1.0.0",
			Author:           "CPA Plugins",
			GitHubRepository: "https://github.com/Cody292/quota-activation",
			Description:      "Quota reset activation management API and scheduler helper.",
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
		{Name: "auto_activate", Type: "boolean", Description: localizedDescription("启用调度器自动配额唤醒；默认 false，因此手动唤醒仍是默认方式。", "Enable scheduler-driven automatic quota activation; default is false, so manual activation remains the default."), DefaultValue: defaults.AutoActivate},
		{Name: "enable_before_activation", Type: "boolean", Description: localizedDescription("显式为 true 时，达到唤醒条件后自动启用已禁用凭证并保持启用。", "When explicitly true, automatically enable disabled credentials that meet activation conditions and keep them enabled."), DefaultValue: defaults.EnableBeforeActivation},
		{Name: "scan_interval", Type: "string", Description: localizedDescription("自动扫描间隔（单位：分钟）。填写纯数字即可，无需带单位；默认 30。", "Automatic scan interval in minutes. Enter a plain number without a unit; default is 30."), DefaultValue: formatDurationMinutes(defaults.ScanInterval)},
		{Name: "activation_request_timeout", Type: "string", Description: localizedDescription("唤醒模型请求超时（单位：秒）。填写纯数字即可，无需带单位；默认 60。", "Activation model request timeout in seconds. Enter a plain number without a unit; default is 60."), DefaultValue: formatDurationSeconds(defaults.ActivationRequestTimeout)},
		{Name: "max_concurrency", Type: "integer", Description: localizedDescription("最大并发唤醒请求数；当前流程预期为 1。", "Maximum concurrent activation requests; the current workflow expects 1."), DefaultValue: defaults.MaxConcurrency},
		{Name: "activation_prompt", Type: "string", Description: localizedDescription("通过 host.model.execute 发送的配额唤醒提示词。", "Prompt sent through host.model.execute for quota activation."), DefaultValue: defaults.ActivationPrompt},
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
