package activator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"quota-activation/internal/host"
)

const activationPriorityBoostFloor = 1000

// boostPriorityForSelection 在 model.execute 前把目标凭证提到 provider 最高 priority 层以上，
// 使 CPA plugin scheduler 候选（仅最高 priority）包含该凭证，从而 nonce 可命中。
// 返回 restore 在唤醒结束后恢复原 priority。
func (a *Activator) boostPriorityForSelection(ctx context.Context, authID string) (restore func(context.Context), err error) {
	noop := func(context.Context) {}
	if a == nil || a.host == nil || strings.TrimSpace(authID) == "" {
		return noop, nil
	}
	files, err := a.host.ListAuthFiles(ctx)
	if err != nil {
		return noop, fmt.Errorf("list auth files for priority boost: %w", safeHostError(err))
	}
	target, ok := findAuthFileByID(files, authID)
	if !ok {
		return noop, nil
	}
	provider := strings.ToLower(strings.TrimSpace(firstNonBlank(target.Provider, target.Type)))
	current := priorityOfAuthFile(target)
	maxPriority := current
	for _, file := range files {
		if strings.ToLower(strings.TrimSpace(firstNonBlank(file.Provider, file.Type))) != provider {
			continue
		}
		if p := priorityOfAuthFile(file); p > maxPriority {
			maxPriority = p
		}
	}
	// 已在最高层（或并列最高）则无需提升。
	if current >= maxPriority {
		return noop, nil
	}
	boosted := maxPriority + 1
	if boosted < activationPriorityBoostFloor {
		boosted = activationPriorityBoostFloor
	}
	// 安全关键：必须从物理凭证完整 JSON 改 priority；没有完整文档时禁止写盘。
	physicalData, physicalName, err := a.loadPhysicalAuthJSON(ctx, target)
	if err != nil {
		return noop, err
	}
	if !looksLikeFullAuthDocument(physicalData) {
		return noop, nil
	}
	current = priorityFromJSON(physicalData, current)
	if current >= maxPriority {
		return noop, nil
	}
	boosted = maxPriority + 1
	if boosted < activationPriorityBoostFloor {
		boosted = activationPriorityBoostFloor
	}
	originalData := append([]byte(nil), physicalData...)
	originalPriority := current
	patched, err := withPriority(physicalData, boosted)
	if err != nil {
		return noop, err
	}
	if !looksLikeFullAuthDocument(patched) {
		return noop, fmt.Errorf("priority boost refused incomplete auth document for %s", authID)
	}
	name := strings.TrimSpace(firstNonBlank(physicalName, target.Name, target.ID, authID))
	if name == "" {
		return noop, nil
	}
	if err := a.host.SaveAuthFile(ctx, name, patched); err != nil {
		return noop, fmt.Errorf("boost auth priority: %w", safeHostError(err))
	}
	restored := false
	return func(restoreCtx context.Context) {
		if restored {
			return
		}
		restored = true
		if restoreCtx == nil {
			restoreCtx = context.Background()
		}
		// 必须用 boost 前完整 JSON 恢复，禁止只写 priority。
		payload, err := withPriority(originalData, originalPriority)
		if err != nil || !looksLikeFullAuthDocument(payload) {
			return
		}
		_ = a.host.SaveAuthFile(restoreCtx, name, payload)
	}, nil
}

func (a *Activator) loadPhysicalAuthJSON(ctx context.Context, file host.AuthFile) ([]byte, string, error) {
	if looksLikeFullAuthDocument(file.Data) {
		return append([]byte(nil), file.Data...), strings.TrimSpace(firstNonBlank(file.Name, file.ID)), nil
	}
	authIndex := strings.TrimSpace(file.AuthIndex)
	if authIndex == "" || a.host == nil {
		return append([]byte(nil), file.Data...), strings.TrimSpace(firstNonBlank(file.Name, file.ID)), nil
	}
	physical, err := a.host.GetAuthFile(ctx, authIndex)
	if err != nil {
		return nil, "", fmt.Errorf("get physical auth file: %w", safeHostError(err))
	}
	return append([]byte(nil), physical.Data...), strings.TrimSpace(firstNonBlank(physical.Name, file.Name, file.ID)), nil
}

func priorityFromJSON(data []byte, fallback int) int {
	if len(data) == 0 {
		return fallback
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return fallback
	}
	raw, ok := document["priority"]
	if !ok {
		return fallback
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return fallback
	}
	return value
}

func findAuthFileByID(files []host.AuthFile, authID string) (host.AuthFile, bool) {
	want := strings.TrimSpace(authID)
	for _, file := range files {
		if strings.TrimSpace(file.ID) == want || strings.TrimSpace(file.Name) == want || strings.TrimSpace(file.AuthIndex) == want {
			return file, true
		}
		if len(file.Data) == 0 {
			continue
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(file.Data, &document); err != nil {
			continue
		}
		if authFileMatches(document, want) {
			return file, true
		}
	}
	return host.AuthFile{}, false
}

func priorityOfAuthFile(file host.AuthFile) int {
	if file.Priority != 0 {
		return file.Priority
	}
	if len(file.Data) == 0 {
		return 0
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(file.Data, &document); err != nil {
		return 0
	}
	raw, ok := document["priority"]
	if !ok {
		return 0
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0
	}
	return value
}

func withPriority(data []byte, priority int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("auth document is empty; refuse priority-only write")
	}
	document := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode auth file: %w", safeError(err))
	}
	if len(document) == 0 {
		return nil, fmt.Errorf("auth document has no fields; refuse priority-only write")
	}
	value, err := json.Marshal(priority)
	if err != nil {
		return nil, fmt.Errorf("encode priority: %w", safeError(err))
	}
	document["priority"] = value
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode auth file: %w", safeError(err))
	}
	return encoded, nil
}

func looksLikeFullAuthDocument(data []byte) bool {
	if len(data) < 64 {
		return false
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return false
	}
	// 必须含 token 字段，避免 REAUTH stub（仅 email/type）被误当完整凭证写 priority。
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		if _, ok := document[key]; ok {
			return true
		}
	}
	return false
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
