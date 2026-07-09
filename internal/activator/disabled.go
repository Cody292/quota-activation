package activator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"quota-activation/internal/host"
)

type restoreFunc func(context.Context) error

func (a *Activator) enableIfNeeded(ctx context.Context, request Request) (restoreFunc, error) {
	if !request.Disabled {
		return nil, nil
	}
	files, err := a.host.ListAuthFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list auth files: %w", safeError(err))
	}
	for _, file := range files {
		updated, ok, err := enableAuthFile(file, request.AuthID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		originalName := file.Name
		originalData := append([]byte(nil), file.Data...)
		if err := a.host.SaveAuthFile(ctx, originalName, updated); err != nil {
			return nil, fmt.Errorf("temporarily enable auth file: %w", safeError(err))
		}
		return restoreOriginalAuthFile(a.host, originalName, originalData), nil
	}
	return nil, ErrAuthFileNotFound
}

func restoreOriginalAuthFile(client host.Client, name string, data []byte) restoreFunc {
	return func(ctx context.Context) error {
		if err := client.SaveAuthFile(ctx, name, data); err != nil {
			return fmt.Errorf("save original auth file: %w", safeError(err))
		}
		return nil
	}
}

func enableAuthFile(file host.AuthFile, authID string) ([]byte, bool, error) {
	if strings.TrimSpace(file.Name) == strings.TrimSpace(authID) {
		return withDisabled(file.Data, false)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(file.Data, &document); err != nil {
		return nil, false, nil
	}
	if !authFileMatches(document, authID) {
		return nil, false, nil
	}
	return withDisabled(file.Data, false)
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
