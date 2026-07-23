package runtime

import (
	"errors"
	"time"

	"quota-activation/internal/config"
	"quota-activation/internal/host"
)

var (
	// ErrInvalidRequest 标识 CPA 调用传入了无法解析的 JSON envelope。
	ErrInvalidRequest = errors.New("runtime: invalid request")
	// ErrShutdown 标识 runtime 已经关闭。
	ErrShutdown = errors.New("runtime: shutdown")
)

// Options 提供 runtime 可注入依赖。
type Options struct {
	Host host.Client
	Now  func() time.Time
}

// RegisterRequest 是 plugin.register 的 JSON 请求形态。
type RegisterRequest struct {
	ConfigYAML string `json:"config_yaml"`
}

// ReconfigureRequest 是 plugin.reconfigure 的 JSON 请求形态。
type ReconfigureRequest struct {
	ConfigYAML string `json:"config_yaml"`
}

// RegisterResult 是 plugin.register 返回给 CPA 的元数据与能力声明。
type RegisterResult struct {
	SchemaVersion int             `json:"schema_version"`
	Metadata      Metadata        `json:"metadata"`
	Capabilities  map[string]bool `json:"capabilities"`
}

// ConfigField 描述 CPA 管理端渲染插件自有配置时使用的字段元数据。
type ConfigField struct {
	Name         string   `json:"Name"`
	Type         string   `json:"Type"`
	Description  string   `json:"Description"`
	EnumValues   []string `json:"EnumValues,omitempty"`
	DefaultValue any      `json:"DefaultValue"`
}

// Metadata 描述插件展示信息，不包含任何敏感配置。
type Metadata struct {
	Name             string        `json:"Name"`
	Version          string        `json:"Version"`
	Author           string        `json:"Author"`
	GitHubRepository string        `json:"GitHubRepository"`
	Description      string        `json:"Description"`
	ConfigFields     []ConfigField `json:"ConfigFields,omitempty"`
}

type runtimeState struct {
	Config config.Config
}
