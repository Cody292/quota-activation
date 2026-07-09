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
	mu         sync.Mutex
	host       host.Client
	now        func() time.Time
	config     config.Config
	sessions   *session.Manager
	store      *state.Store
	activator  *activator.Activator
	management *management.Handler
	picker     scheduler.Picker
	shutdown   bool
}

// New 创建未注册 runtime；首次 register/reconfigure 成功后才启用依赖。
func New(options Options) *Runtime {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Runtime{host: options.Host, now: now, config: config.Default()}
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
	r.config = cfg
	r.sessions = sessions
	r.store = store
	r.activator = activation
	r.management = manager
	r.picker = scheduler.NewPicker(sessions)
	return nil
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
	return []ConfigField{
		{Name: "activation_models", Type: "object", Description: localizedDescription("Codex 与 Antigravity 模型组使用的唤醒模型映射；未填写时使用安全默认值。", "Activation model mapping for Codex and Antigravity model groups; safe defaults are used when omitted.")},
		{Name: "activation_models.antigravity.enable_gemini", Type: "boolean", Description: localizedDescription("是否启用 Antigravity Gemini 模型组唤醒。", "Whether Antigravity Gemini model-group activation is enabled.")},
		{Name: "activation_models.antigravity.enable_claude_gpt", Type: "boolean", Description: localizedDescription("是否启用 Antigravity Claude/GPT 模型组唤醒。", "Whether Antigravity Claude/GPT model-group activation is enabled.")},
		{Name: "auto_activate", Type: "boolean", Description: localizedDescription("启用调度器自动配额唤醒；默认 false，因此手动唤醒仍是默认方式。", "Enable scheduler-driven automatic quota activation; default is false, so manual activation remains the default.")},
		{Name: "scan_interval", Type: "string", Description: localizedDescription("自动扫描间隔，按 Go time.ParseDuration 解析，例如 30m。", "Automatic scan interval parsed by Go time.ParseDuration, for example 30m.")},
		{Name: "max_probe_interval", Type: "string", Description: localizedDescription("最大配额探测间隔，按 Go time.ParseDuration 解析，例如 30m。", "Maximum quota probe interval parsed by Go time.ParseDuration, for example 30m.")},
		{Name: "min_probe_interval", Type: "string", Description: localizedDescription("最小配额探测间隔，按 Go time.ParseDuration 解析，例如 5m。", "Minimum quota probe interval parsed by Go time.ParseDuration, for example 5m.")},
		{Name: "activation_request_timeout", Type: "string", Description: localizedDescription("唤醒模型请求超时，按 Go time.ParseDuration 解析，例如 60s。", "Activation model request timeout parsed by Go time.ParseDuration, for example 60s.")},
		{Name: "max_concurrency", Type: "integer", Description: localizedDescription("最大并发唤醒请求数；当前流程预期为 1。", "Maximum concurrent activation requests; the current workflow expects 1.")},
		{Name: "activation_prompt", Type: "string", Description: localizedDescription("通过 host.model.execute 发送的配额唤醒提示词。", "Prompt sent through host.model.execute for quota activation.")},
		{Name: "state_path", Type: "string", Description: localizedDescription("用于脱敏唤醒记录的相对状态文件路径。", "Relative state file path used for sanitized activation records.")},
		{Name: "enable_before_activation", Type: "boolean", Description: localizedDescription("显式为 true 时，唤醒前临时启用已禁用凭证并在结束后恢复。", "When explicitly true, temporarily enable disabled credentials before activation and restore them afterward.")},
	}
}

func localizedDescription(chinese string, english string) string {
	return "中文：" + chinese + "\nEnglish: " + english
}
