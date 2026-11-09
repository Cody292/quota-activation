package activator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"quota-activation/internal/detector"
	"quota-activation/internal/host"
	"quota-activation/internal/state"
)

const antigravityAuthModelsRetryExtraAttempts = 2
const antigravityAuthModelsRetryDelay = 100 * time.Millisecond

func (a *Activator) authorizeAntigravityModel(ctx context.Context, request Request, enabledNow bool) (string, error) {
	if a == nil || a.host == nil {
		return "", fmt.Errorf("available model is required for auth %s: %w", request.AuthID, ErrMissingDependency)
	}
	authID := strings.TrimSpace(request.AuthID)
	if authID == "" {
		return "", fmt.Errorf("available model is required: missing auth_id")
	}

	files, err := a.host.ListAuthFiles(ctx)
	if err != nil {
		return "", fmt.Errorf("list auth files for available models: %w", safeHostError(err))
	}
	target, ok := findMatchingAuthFile(files, authID)
	if !ok {
		return "", fmt.Errorf("available model is required for auth %s", authID)
	}
	actualIndex := strings.TrimSpace(target.AuthIndex)
	canonicalID := strings.TrimSpace(target.ID)
	if canonicalID == "" {
		canonicalID = authID
	}
	if actualIndex == "" && canonicalID == "" {
		return "", fmt.Errorf("available model is required for auth %s", authID)
	}

	queryKey := actualIndex
	queryLabel := actualIndex
	if actualIndex == "" {
		queryKey = ""
		queryLabel = canonicalID
	}

	maxAttempts := 1
	if enabledNow {
		maxAttempts += antigravityAuthModelsRetryExtraAttempts
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if !a.sleepCtx(ctx, antigravityAuthModelsRetryDelay) {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return "", fmt.Errorf("available model is required for auth %s: %w", authID, ctxErr)
				}
				return "", fmt.Errorf("available model is required for auth %s", authID)
			}
		}

		credentials, err := host.GetAuthModels(ctx, a.host, queryKey)
		if err != nil {
			return "", fmt.Errorf("get auth models for %s: %w", queryLabel, safeAuthModelsError(err))
		}

		var matchingCreds []host.AuthCredentialModels
		for _, cred := range credentials {
			var matchedCandidate bool
			if actualIndex != "" {
				matchedCandidate = strings.TrimSpace(cred.AuthIndex) == actualIndex
			} else {
				matchedCandidate = strings.TrimSpace(cred.ID) == canonicalID
			}
			if matchedCandidate {
				matchingCreds = append(matchingCreds, cred)
			}
		}

		if len(matchingCreds) > 1 {
			return "", fmt.Errorf("available model is required for auth %s: ambiguous credentials", authID)
		}
		if len(matchingCreds) == 0 {
			continue
		}

		targetCred := matchingCreds[0]
		if !strings.EqualFold(strings.TrimSpace(targetCred.Provider), string(detector.ProviderAntigravity)) {
			return "", fmt.Errorf("available model is required for auth %s", authID)
		}

		var registeredModels []string
		for _, m := range targetCred.Models {
			trimmed := strings.TrimSpace(m)
			if trimmed != "" {
				registeredModels = append(registeredModels, trimmed)
			}
		}
		if len(registeredModels) == 0 {
			continue
		}

		var enabledModels []string
		for _, m := range registeredModels {
			if !isModelDisabled(m) {
				enabledModels = append(enabledModels, m)
			}
		}
		if len(enabledModels) == 0 {
			return "", fmt.Errorf("available model is required for auth %s", authID)
		}

		requestedModel := strings.TrimSpace(request.Model)
		if requestedModel != "" {
			if isModelDisabled(requestedModel) {
				return "", fmt.Errorf("available model is required for auth %s: model %s is disabled", authID, requestedModel)
			}
			if request.ModelGroup != detector.ModelGroupNone {
				group, ok := detector.InferModelGroup(requestedModel)
				if !ok || group != request.ModelGroup {
					return "", fmt.Errorf("available model is required for auth %s: model %s group mismatch", authID, requestedModel)
				}
			}
			for _, m := range enabledModels {
				if m == requestedModel {
					return m, nil
				}
			}
			return "", fmt.Errorf("available model is required for auth %s: model %s not authorized", authID, requestedModel)
		}

		for _, m := range enabledModels {
			if request.ModelGroup != detector.ModelGroupNone {
				group, ok := detector.InferModelGroup(m)
				if !ok || group != request.ModelGroup {
					continue
				}
			}
			return m, nil
		}
		return "", fmt.Errorf("available model is required for auth %s", authID)
	}

	if enabledNow {
		return "", fmt.Errorf("available model is required for auth %s: registry models empty after retry", authID)
	}
	return "", fmt.Errorf("available model is required for auth %s", authID)
}

func isModelDisabled(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(lower, "disabled")
}

func findMatchingAuthFile(files []host.AuthFile, authID string) (host.AuthFile, bool) {
	target, ok := findAuthFileByID(files, authID)
	if ok {
		return target, true
	}
	want := strings.TrimSpace(authID)
	if want == "" {
		return host.AuthFile{}, false
	}
	for _, file := range files {
		document := decodeAuthFileModels(file)
		if want == firstNonBlankModelField(file.ID, file.Name, document.Name, file.AuthIndex, document.AuthID, document.AuthIDUpper, document.ID) {
			return file, true
		}
	}
	return host.AuthFile{}, false
}

func safeAuthModelsError(err error) error {
	if err == nil {
		return nil
	}
	var statusErr *host.HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return safeError(err)
	}
	if isNetworkFailure(err) {
		return redactedError{message: ErrNetworkFailure.Error(), cause: errors.Join(ErrNetworkFailure, err)}
	}
	if errors.Is(err, host.ErrAuthModelsUnsupported) {
		return redactedError{message: "host callback host.auth.models unsupported", cause: err}
	}
	message := state.Redact(err.Error())
	if message != "" && strings.Contains(message, "host callback host.auth.models") {
		clean := message
		if strings.Contains(clean, "host.model.execute") || strings.Contains(clean, "host model execute") {
			clean = "host callback host.auth.models query failed"
		}
		return redactedError{message: clean, cause: err}
	}
	if message != "" && strings.Contains(strings.ToLower(message), "unsupported") {
		return redactedError{message: "host callback host.auth.models unsupported", cause: err}
	}
	return redactedError{message: "model-list query failed", cause: err}
}
