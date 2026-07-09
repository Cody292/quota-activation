package host

import (
	"context"
	"errors"
	"fmt"
)

// ErrModelExecuteStatus 表示 host.model.execute 返回了非 2xx HTTP 状态。
var ErrModelExecuteStatus = errors.New("host model execute status")

// Client 是配额激活逻辑依赖的最小 CPA 宿主适配接口。
type Client interface {
	ModelExecute(ctx context.Context, request ModelExecuteRequest) (ModelExecuteResponse, error)
	ListAuthFiles(ctx context.Context) ([]AuthFile, error)
	SaveAuthFile(ctx context.Context, name string, data []byte) error
}

// ModelExecuteRequest 表示可安全脱敏的 host.model.execute 输入形状。
type ModelExecuteRequest struct {
	Model   string
	Headers map[string][]string
	Body    []byte
}

// ModelExecuteResponse 表示 host.model.execute 返回的 HTTP 形状结果。
type ModelExecuteResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
}

// AuthFile 表示宿主凭证存储中的一个凭证元数据或原始凭证文档。
type AuthFile struct {
	Name      string `json:"name"`
	AuthIndex string `json:"auth_index"`
	Account   string `json:"account,omitempty"`
	Email     string `json:"email,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Type      string `json:"type,omitempty"`
	Disabled  bool   `json:"disabled"`
	Data      []byte `json:"data,omitempty"`
}

// HTTPStatusError 暴露 HTTP 状态码，但不包含请求体或响应体。
type HTTPStatusError struct {
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("host model execute returned HTTP status %d", e.StatusCode)
}

func (e *HTTPStatusError) Is(target error) bool {
	return target == ErrModelExecuteStatus
}

// ResponseOrStatusError 返回 2xx 响应，并将非 2xx 状态包装为状态错误。
func ResponseOrStatusError(response ModelExecuteResponse) (ModelExecuteResponse, error) {
	if IsHTTPSuccess(response.StatusCode) {
		return response, nil
	}
	return ModelExecuteResponse{}, fmt.Errorf("model execute: %w", &HTTPStatusError{StatusCode: response.StatusCode})
}

// IsHTTPSuccess 判断 statusCode 是否处于 HTTP 2xx 成功范围。
func IsHTTPSuccess(statusCode int) bool {
	return statusCode >= 200 && statusCode <= 299
}
