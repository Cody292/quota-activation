package activator

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// AntigravityActivationURL 是 Antigravity 官方 generateContent 唤醒端点。
// 与 Codex chatgpt.com/backend-api/codex/responses 完全独立，禁止混用。
const AntigravityActivationURL = "https://daily-cloudcode-pa.googleapis.com/v1internal:generateContent"

const (
	antigravityUserAgent       = "antigravity"
	antigravityRequestType     = "agent"
	antigravityProjectPref     = "projects/"
	antigravityClientUserAgent = "antigravity/hub/2.24.0 (aidev_client; os_type=darwin; arch=arm64; cl=1000335682)"
)

// ErrAntigravityProtocol 表示 Antigravity 协议构建失败（品牌名保留英文，其余中文）。
var ErrAntigravityProtocol = errors.New("Antigravity协议无效")

// antigravityActivationBody 是官方 generateContent 信封。
// 禁止混入 OpenAI messages/choices 或 Codex input/store。
type antigravityActivationBody struct {
	Project     string                  `json:"project"`
	Model       string                  `json:"model"`
	UserAgent   string                  `json:"userAgent"`
	RequestType string                  `json:"requestType"`
	RequestID   string                  `json:"requestId"`
	Request     antigravityInnerRequest `json:"request"`
}

type antigravityInnerRequest struct {
	SessionID        string                       `json:"sessionId"`
	Contents         []antigravityContent         `json:"contents"`
	GenerationConfig *antigravityGenerationConfig `json:"generationConfig,omitempty"`
}

type antigravityGenerationConfig struct {
	ThinkingConfig *antigravityThinkingConfig `json:"thinkingConfig,omitempty"`
}

type antigravityThinkingConfig struct {
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
}

type antigravityContent struct {
	Role  string            `json:"role"`
	Parts []antigravityPart `json:"parts"`
}

type antigravityPart struct {
	Text string `json:"text"`
}

// BuildAntigravityProtocol 构建 Antigravity 直连唤醒请求。
// 缺 token / model / project 必须在构建期失败，不得发出 HTTP。
func BuildAntigravityProtocol(material AuthMaterial, model, prompt string) (ProtocolRequest, error) {
	token := strings.TrimSpace(material.AccessToken)
	if token == "" {
		return ProtocolRequest{}, fmt.Errorf("%w：缺少访问令牌", ErrAntigravityProtocol)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return ProtocolRequest{}, fmt.Errorf("%w：缺少模型名称", ErrAntigravityProtocol)
	}
	projectID, err := normalizeAntigravityProjectID(material.ProjectID)
	if err != nil {
		return ProtocolRequest{}, err
	}
	model, thinkingLevel := normalizeAntigravityModel(model)

	requestID, err := newAntigravityRequestID()
	if err != nil {
		return ProtocolRequest{}, fmt.Errorf("%w：生成请求标识失败", ErrAntigravityProtocol)
	}
	sessionID, err := newAntigravitySessionID()
	if err != nil {
		return ProtocolRequest{}, fmt.Errorf("%w：生成会话标识失败", ErrAntigravityProtocol)
	}

	request := antigravityInnerRequest{
		SessionID: sessionID,
		Contents: []antigravityContent{
			{
				Role:  "user",
				Parts: []antigravityPart{{Text: prompt}},
			},
		},
	}
	if thinkingLevel != "" {
		request.GenerationConfig = &antigravityGenerationConfig{
			ThinkingConfig: &antigravityThinkingConfig{ThinkingLevel: thinkingLevel},
		}
	}
	body, err := json.Marshal(antigravityActivationBody{
		Project:     projectID,
		Model:       model,
		UserAgent:   antigravityUserAgent,
		RequestType: antigravityRequestType,
		RequestID:   requestID,
		Request:     request,
	})
	if err != nil {
		return ProtocolRequest{}, fmt.Errorf("%w：编码请求失败", ErrAntigravityProtocol)
	}

	headers := map[string][]string{
		"Accept":        {"application/json"},
		"Authorization": {"Bearer " + token},
		"Content-Type":  {"application/json"},
		"User-Agent":    {antigravityClientUserAgent},
	}

	return ProtocolRequest{
		Method:  http.MethodPost,
		URL:     AntigravityActivationURL,
		Headers: headers,
		Body:    body,
	}, nil
}

func normalizeAntigravityProjectID(raw string) (string, error) {
	projectID := strings.TrimSpace(raw)
	if strings.HasPrefix(projectID, antigravityProjectPref) {
		projectID = strings.TrimPrefix(projectID, antigravityProjectPref)
	}
	if projectID == "" {
		return "", fmt.Errorf("%w：缺少项目", ErrAntigravityProtocol)
	}
	return projectID, nil
}

func newAntigravityRequestID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	buf[6] = buf[6]&0x0f | 0x40
	buf[8] = buf[8]&0x3f | 0x80
	uuid := hex.EncodeToString(buf[:])
	return "agent-" + uuid[:8] + "-" + uuid[8:12] + "-" + uuid[12:16] + "-" + uuid[16:20] + "-" + uuid[20:], nil
}

func newAntigravitySessionID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	buf[0] &= 0x7f
	sessionID := binary.BigEndian.Uint64(buf[:]) | 1
	return "-" + strconv.FormatUint(sessionID, 10), nil
}

func normalizeAntigravityModel(model string) (string, string) {
	lower := strings.ToLower(model)
	if !strings.HasPrefix(lower, "gemini-3.8-flash-") {
		return model, ""
	}
	_, level, ok := strings.Cut(lower, "gemini-3.8-flash-")
	if !ok {
		return model, ""
	}
	switch strings.ToLower(level) {
	case "low", "medium", "high":
		return model, level
	default:
		return model, ""
	}
}
