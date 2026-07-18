package activator

import (
	"errors"
	"time"

	"quota-activation/internal/config"
	"quota-activation/internal/detector"
	"quota-activation/internal/host"
	"quota-activation/internal/session"
	"quota-activation/internal/state"
)

var (
	// ErrBusy 表示已有激活流程正在执行，本次请求不会并发进入宿主调用。
	ErrBusy = errors.New("activator: busy")
	// ErrDisabledCredential 历史兼容错误：禁用凭证不再因此阻断手动/统一唤醒。
	// 管理层仍可能匹配该错误码，但 Activate 路径不再返回它。
	ErrDisabledCredential = errors.New("activator: disabled credential")
	// ErrInvalidRequest 表示激活请求缺少必要的目标、周期或模型信息。
	ErrInvalidRequest = errors.New("activator: invalid request")
	// ErrMissingDependency 表示 Activator 缺少必要内部依赖。
	ErrMissingDependency = errors.New("activator: missing dependency")
	// ErrAuthFileNotFound 表示自动启用禁用凭证时没有找到匹配的宿主凭证文档。
	ErrAuthFileNotFound = errors.New("activator: auth file not found")
)

// Status 表示单次激活的脱敏执行状态。
type Status string

const (
	// StatusSuccess 表示 host.model.execute 返回 HTTP 2xx 且状态写入完成。
	StatusSuccess Status = "success"
	// StatusFailed 表示 host.model.execute、自动启用或状态写入前的流程失败。
	StatusFailed Status = "failed"
	// StatusSkipped 表示目标不允许激活，流程未调用宿主模型接口。
	StatusSkipped Status = "skipped"
	// StatusBusy 表示串行执行器正忙，本次请求未进入激活流程。
	StatusBusy Status = "busy"
)

// Options 定义 Activator 的内部依赖和稳定配置。
type Options struct {
	Host     host.Client
	Sessions *session.Manager
	State    *state.Store
	Config   config.Config
	Now      func() time.Time
}

// Request 描述一次纯内部凭证激活请求。
type Request struct {
	AuthID     string
	Provider   detector.Provider
	ModelGroup detector.ModelGroup
	Window     detector.Window
	CycleKey   detector.CycleKey
	Model      string
	Prompt     string
	Disabled   bool
	ObservedAt time.Time
	ResetAt    time.Time
}

// Result 是一次激活流程的脱敏结果。
type Result struct {
	AuthID           string    `json:"auth_id"`
	Provider         string    `json:"provider"`
	Window           string    `json:"window"`
	CycleKey         string    `json:"cycle_key"`
	Status           Status    `json:"status"`
	Success          bool      `json:"success"`
	Nonce            string    `json:"nonce,omitempty"`
	HTTPStatus       int       `json:"http_status,omitempty"`
	TemporaryEnabled bool      `json:"temporary_enabled,omitempty"`
	RestoredDisabled bool      `json:"restored_disabled,omitempty"`
	ObservedAt       time.Time `json:"observed_at"`
	ResetAt          time.Time `json:"reset_at"`
	LastError        string    `json:"last_error,omitempty"`
}
