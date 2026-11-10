package config

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// OAuthModelAlias 描述宿主 OAuth 模型别名。
type OAuthModelAlias struct {
	Name  string `json:"name"`
	Alias string `json:"alias"`
	Fork  bool   `json:"fork"`
}

// OAuthModelAliases 按提供商隔离宿主模型别名。
type OAuthModelAliases map[string][]OAuthModelAlias

func parseOAuthModelAliases(text string) (OAuthModelAliases, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, invalid("oauth_model_aliases", "必须是有效的 JSON 对象")
	}
	result := make(OAuthModelAliases)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, invalid("oauth_model_aliases", "无法读取提供商")
		}
		channel, ok := key.(string)
		if !ok {
			return nil, invalid("oauth_model_aliases", "提供商必须是字符串")
		}
		var entries []OAuthModelAlias
		if err := decoder.Decode(&entries); err != nil || entries == nil {
			return nil, invalid("oauth_model_aliases", "提供商的映射必须是数组")
		}
		provider := strings.ToLower(strings.TrimSpace(channel))
		if provider == "" {
			return nil, invalid("oauth_model_aliases", "提供商不能为空")
		}
		if _, exists := result[provider]; exists {
			return nil, invalid("oauth_model_aliases", "提供商名称重复")
		}
		seen := make(map[string]struct{}, len(entries))
		clean := make([]OAuthModelAlias, 0, len(entries))
		for _, entry := range entries {
			entry.Name = strings.TrimSpace(entry.Name)
			entry.Alias = strings.TrimSpace(entry.Alias)
			if entry.Name == "" || entry.Alias == "" {
				return nil, invalid("oauth_model_aliases", "模型名称和别名不能为空")
			}
			if strings.EqualFold(entry.Name, entry.Alias) {
				continue
			}
			key := strings.ToLower(entry.Alias)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			clean = append(clean, entry)
		}
		for _, entry := range clean {
			if _, exists := seen[strings.ToLower(entry.Name)]; exists {
				return nil, invalid("oauth_model_aliases", "模型名称与别名冲突")
			}
			if index := strings.LastIndex(entry.Name, "("); index > 0 && strings.HasSuffix(entry.Name, ")") {
				if _, exists := seen[strings.ToLower(entry.Name[:index])]; exists {
					return nil, invalid("oauth_model_aliases", "模型名称与别名冲突")
				}
			}
		}
		result[provider] = clean
	}
	if _, err := decoder.Token(); err != nil {
		return nil, invalid("oauth_model_aliases", "JSON 对象不完整")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, invalid("oauth_model_aliases", "JSON 对象后存在多余内容")
	}
	return result, nil
}

// Resolve 返回提供商对应的上游模型，未映射时保留原值。
func (aliases OAuthModelAliases) Resolve(provider, model string) string {
	requested := strings.TrimSpace(model)
	entries := aliases[strings.ToLower(strings.TrimSpace(provider))]
	base, suffix := requested, ""
	if index := strings.LastIndex(requested, "("); index > 0 && strings.HasSuffix(requested, ")") {
		base, suffix = requested[:index], requested[index:]
	}
	for _, candidate := range []string{requested, base} {
		for _, entry := range entries {
			if strings.EqualFold(entry.Alias, candidate) {
				if strings.EqualFold(entry.Name, base) {
					return requested
				}
				if suffix != "" && suffix != "()" && !(strings.Contains(entry.Name, "(") && strings.HasSuffix(entry.Name, ")")) {
					return entry.Name + suffix
				}
				return entry.Name
			}
		}
	}
	return requested
}

// RegisteredModel 在已注册名称中匹配输入，不根据映射推断凭证权限。
func (aliases OAuthModelAliases) RegisteredModel(request ModelRegistrationRequest) (string, bool, error) {
	provider, requested, registered := request.Provider, request.Model, request.Registered
	requested = strings.TrimSpace(requested)
	for _, name := range registered {
		if strings.EqualFold(strings.TrimSpace(name), requested) {
			return strings.TrimSpace(name), true, nil
		}
	}
	var matched string
	for _, entry := range aliases[strings.ToLower(strings.TrimSpace(provider))] {
		if !strings.EqualFold(entry.Name, requested) {
			continue
		}
		for _, name := range registered {
			if !strings.EqualFold(strings.TrimSpace(name), entry.Alias) {
				continue
			}
			if !strings.EqualFold(aliases.Resolve(provider, name), requested) {
				return "", false, fmt.Errorf("%w: oauth_model_aliases 模型映射存在歧义", ErrInvalidConfig)
			}
			if matched == "" {
				matched = strings.TrimSpace(name)
			} else if !strings.EqualFold(matched, name) {
				return "", false, fmt.Errorf("%w: oauth_model_aliases 存在多个已注册别名，请指定别名", ErrInvalidConfig)
			}
		}
	}
	return matched, matched != "", nil
}

// ModelRegistrationRequest 包含凭证已注册模型和需要匹配的名称。
type ModelRegistrationRequest struct {
	Provider   string
	Model      string
	Registered []string
}

// Redacted 返回仅包含可安全展示模型标识的映射副本。
func (aliases OAuthModelAliases) Redacted(redact func(string) string) OAuthModelAliases {
	result := make(OAuthModelAliases, len(aliases))
	for provider, entries := range aliases {
		clean := make([]OAuthModelAlias, len(entries))
		for i, entry := range entries {
			clean[i] = OAuthModelAlias{Name: redact(entry.Name), Alias: redact(entry.Alias), Fork: entry.Fork}
		}
		result[redact(provider)] = clean
	}
	return result
}
