package activator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"quota-activation/internal/host"
)

func (a *Activator) credentialEnableLock(authID string) chan struct{} {
	key := strings.TrimSpace(authID)
	a.enableLocksMu.Lock()
	defer a.enableLocksMu.Unlock()
	if a.enableLocks == nil {
		a.enableLocks = make(map[string]chan struct{})
	}
	ch, ok := a.enableLocks[key]
	if !ok {
		ch = make(chan struct{}, 1)
		a.enableLocks[key] = ch
	}
	return ch
}

func (a *Activator) resolveCredentialLockKey(ctx context.Context, authID string) string {
	want := strings.TrimSpace(authID)
	if want == "" || a == nil || a.host == nil {
		return want
	}
	files, err := a.host.ListAuthFiles(ctx)
	if err != nil {
		return want
	}
	for _, file := range files {
		if strings.TrimSpace(file.ID) == want || strings.TrimSpace(file.Name) == want || strings.TrimSpace(file.AuthIndex) == want {
			return strings.TrimSpace(firstNonBlank(file.Name, file.ID, file.AuthIndex, want))
		}
		if len(file.Data) > 0 {
			var document map[string]json.RawMessage
			if err := json.Unmarshal(file.Data, &document); err == nil && authFileMatches(document, want) {
				return strings.TrimSpace(firstNonBlank(file.Name, file.ID, file.AuthIndex, want))
			}
		}
	}
	return want
}

func (a *Activator) acquireCredentialEnableLock(ctx context.Context, authID string) (func(), error) {
	if a == nil {
		return func() {}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ch := a.credentialEnableLock(a.resolveCredentialLockKey(ctx, authID))
	select {
	case ch <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-ch
			return nil, err
		}
		return func() {
			<-ch
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Activator) enableIfNeeded(ctx context.Context, request Request) (bool, error) {
	if !request.Disabled {
		return false, nil
	}
	release, err := a.acquireCredentialEnableLock(ctx, request.AuthID)
	if err != nil {
		return false, err
	}
	defer release()

	if err := ctx.Err(); err != nil {
		return false, err
	}

	files, err := a.host.ListAuthFiles(ctx)
	if err != nil {
		return false, fmt.Errorf("list auth files: %w", safeError(err))
	}
	for _, file := range files {
		updated, ok, err := enableAuthFile(file, request.AuthID)
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		if len(updated) > 0 {
			saveName := strings.TrimSpace(firstNonBlank(file.Name, file.ID))
			if err := a.host.SaveAuthFile(ctx, saveName, updated); err != nil {
				return false, fmt.Errorf("enable auth file: %w", safeError(err))
			}
		}
		return true, nil
	}
	return false, ErrAuthFileNotFound
}

func enableAuthFile(file host.AuthFile, authID string) ([]byte, bool, error) {
	want := strings.TrimSpace(authID)
	aliasMatch := want != "" && (strings.TrimSpace(file.Name) == want || strings.TrimSpace(file.ID) == want || strings.TrimSpace(file.AuthIndex) == want)

	var document map[string]json.RawMessage
	if err := json.Unmarshal(file.Data, &document); err != nil {
		if aliasMatch {
			return nil, false, fmt.Errorf("decode auth file: %w", safeError(err))
		}
		return nil, false, nil
	}
	if !aliasMatch && !authFileMatches(document, want) {
		return nil, false, nil
	}
	if !isAuthDocumentDisabled(document) {
		return nil, true, nil
	}
	updated, _, err := withDisabled(file.Data, false)
	if err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

func isAuthDocumentDisabled(document map[string]json.RawMessage) bool {
	if document == nil {
		return false
	}
	raw, ok := document["disabled"]
	if !ok {
		return false
	}
	var disabled bool
	if err := json.Unmarshal(raw, &disabled); err == nil {
		return disabled
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.EqualFold(strings.TrimSpace(s), "true")
	}
	return false
}

func authFileMatches(document map[string]json.RawMessage, authID string) bool {
	for _, key := range []string{"auth_id", "authID", "id", "name"} {
		raw, ok := document[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err == nil && strings.TrimSpace(value) == strings.TrimSpace(authID) {
			return true
		}
	}
	return false
}

func withDisabled(data []byte, disabled bool) ([]byte, bool, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, false, fmt.Errorf("decode auth file: %w", safeError(err))
	}
	value, err := json.Marshal(disabled)
	if err != nil {
		return nil, false, fmt.Errorf("encode disabled flag: %w", safeError(err))
	}
	document["disabled"] = value
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, false, fmt.Errorf("encode auth file: %w", safeError(err))
	}
	return encoded, true, nil
}
