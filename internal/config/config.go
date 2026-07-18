package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrInvalidConfig 标识插件配置解析或校验失败。
	ErrInvalidConfig = errors.New("配置无效")
)

// Config 是 quota-activation 插件配置解析后的稳定形态。
type Config struct {
	AutoActivate             bool
	ScanInterval             time.Duration
	MaxProbeInterval         time.Duration
	MinProbeInterval         time.Duration
	ActivationRequestTimeout time.Duration
	MaxConcurrency           int
	ActivationPrompt         string
	StatePath                string
	EnableBeforeActivation   bool
	ActivationModels         ActivationModels
}

// ActivationModels 定义不同 provider 唤醒请求必须显式配置的模型。
type ActivationModels struct {
	Codex       string
	Antigravity AntigravityActivationModels
}

// AntigravityActivationModels 定义 Antigravity 不同模型组的唤醒模型。
type AntigravityActivationModels struct {
	Gemini          string
	ClaudeGPT       string
	EnableGemini    bool
	EnableClaudeGPT bool
}

type rawConfig struct {
	Enabled                  *bool                `json:"enabled"`
	Priority                 *int                 `json:"priority"`
	AutoActivate             *bool                `json:"auto_activate"`
	ScanInterval             *string              `json:"scan_interval"`
	MaxProbeInterval         *string              `json:"max_probe_interval"`
	MinProbeInterval         *string              `json:"min_probe_interval"`
	ActivationRequestTimeout *string              `json:"activation_request_timeout"`
	MaxConcurrency           *int                 `json:"max_concurrency"`
	ActivationPrompt         *string              `json:"activation_prompt"`
	StatePath                *string              `json:"state_path"`
	EnableBeforeActivation   *bool                `json:"enable_before_activation"`
	ActivationModels         *rawActivationModels `json:"activation_models"`
}

type rawActivationModels struct {
	Codex       *string                    `json:"codex"`
	Antigravity *rawAntigravityModelsGroup `json:"antigravity"`
}

type rawAntigravityModelsGroup struct {
	Gemini          *string `json:"gemini"`
	ClaudeGPT       *string `json:"claude_gpt"`
	EnableGemini    *bool   `json:"enable_gemini"`
	EnableClaudeGPT *bool   `json:"enable_claude_gpt"`
}

// Default 返回除唤醒模型外的稳定默认配置。
func Default() Config {
	return Config{
		AutoActivate:             false,
		ScanInterval:             30 * time.Minute,
		MaxProbeInterval:         30 * time.Minute,
		MinProbeInterval:         5 * time.Minute,
		ActivationRequestTimeout: time.Minute,
		MaxConcurrency:           1,
		ActivationPrompt:         "quota activation ping",
		StatePath:                "quota-activation/state.json",
		EnableBeforeActivation:   false,
		ActivationModels: ActivationModels{
			Antigravity: AntigravityActivationModels{
				EnableGemini:    true,
				EnableClaudeGPT: true,
			},
		},
	}
}

// Parse 将 CPA 传入的插件配置 JSON 字节解析为已校验配置。
func Parse(data []byte) (Config, error) {
	raw, err := decodeRaw(data)
	if err != nil {
		return Config{}, fmt.Errorf("解析配置: %w", err)
	}
	cfg, err := raw.apply(Default())
	if err != nil {
		return Config{}, fmt.Errorf("校验配置: %w", err)
	}
	return cfg, nil
}

func decodeRaw(data []byte) (rawConfig, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return rawConfig{}, nil
	}
	var raw rawConfig
	if trimmed[0] == '{' {
		decoder := json.NewDecoder(strings.NewReader(trimmed))
		if err := decoder.Decode(&raw); err != nil {
			return rawConfig{}, invalid("config", fmt.Sprintf("必须匹配配置结构: %v", err))
		}
		return raw, nil
	}

	yamlMap, err := parseYAMLMap(trimmed)
	if err != nil {
		return rawConfig{}, err
	}
	encoded, err := json.Marshal(yamlMap)
	if err != nil {
		return rawConfig{}, invalid("config", "YAML 编码错误")
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	if err := decoder.Decode(&raw); err != nil {
		return rawConfig{}, invalid("config", fmt.Sprintf("YAML 不匹配配置结构: %v", err))
	}
	return raw, nil
}

func (raw rawConfig) apply(cfg Config) (Config, error) {
	if raw.AutoActivate != nil {
		cfg.AutoActivate = *raw.AutoActivate
	}
	if raw.ActivationPrompt != nil {
		cfg.ActivationPrompt = *raw.ActivationPrompt
	}
	if raw.StatePath != nil {
		cfg.StatePath = *raw.StatePath
	}
	if raw.EnableBeforeActivation != nil {
		cfg.EnableBeforeActivation = *raw.EnableBeforeActivation
	}
	if raw.MaxConcurrency != nil {
		cfg.MaxConcurrency = *raw.MaxConcurrency
	}
	if cfg.MaxConcurrency < 1 {
		return Config{}, invalid("max_concurrency", "必须大于等于 1")
	}
	durationFields := []struct {
		name   string
		raw    *string
		target *time.Duration
	}{
		{"scan_interval", raw.ScanInterval, &cfg.ScanInterval},
		{"max_probe_interval", raw.MaxProbeInterval, &cfg.MaxProbeInterval},
		{"min_probe_interval", raw.MinProbeInterval, &cfg.MinProbeInterval},
		{"activation_request_timeout", raw.ActivationRequestTimeout, &cfg.ActivationRequestTimeout},
	}
	for _, field := range durationFields {
		if field.raw == nil {
			continue
		}
		parsed, err := parseDuration(field.name, *field.raw)
		if err != nil {
			return Config{}, err
		}
		*field.target = parsed
	}
	models, err := raw.ActivationModels.parse(cfg.ActivationModels)
	if err != nil {
		return Config{}, err
	}
	cfg.ActivationModels = models
	return cfg, nil
}

func (raw *rawActivationModels) parse(base ActivationModels) (ActivationModels, error) {
	models := base
	if raw != nil && raw.Codex != nil {
		models.Codex = strings.TrimSpace(*raw.Codex)
	}
	if raw != nil && raw.Antigravity != nil {
		models.Antigravity = raw.Antigravity.parse(models.Antigravity)
	}
	return models, nil
}

func (raw rawAntigravityModelsGroup) parse(base AntigravityActivationModels) AntigravityActivationModels {
	models := base
	if raw.Gemini != nil {
		models.Gemini = strings.TrimSpace(*raw.Gemini)
	}
	if raw.ClaudeGPT != nil {
		models.ClaudeGPT = strings.TrimSpace(*raw.ClaudeGPT)
	}
	if raw.EnableGemini != nil {
		models.EnableGemini = *raw.EnableGemini
	}
	if raw.EnableClaudeGPT != nil {
		models.EnableClaudeGPT = *raw.EnableClaudeGPT
	}
	return models
}

func parseDuration(field string, raw string) (time.Duration, error) {
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, invalid(field, "必须是 time.ParseDuration 可解析的字符串")
	}
	return parsed, nil
}

func invalid(field string, reason string) error {
	return fmt.Errorf("%w: %s %s", ErrInvalidConfig, field, reason)
}
