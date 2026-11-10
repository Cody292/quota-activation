package activator

import (
	"fmt"
	"strings"

	"quota-activation/internal/detector"
)

func (a *Activator) normalizeRequest(request Request) (Request, error) {
	normalized := request
	normalized.AuthID = strings.TrimSpace(request.AuthID)
	normalized.Model = strings.TrimSpace(request.Model)
	normalized.Prompt = strings.TrimSpace(request.Prompt)
	if normalized.Prompt == "" {
		normalized.Prompt = strings.TrimSpace(a.config.ActivationPrompt)
	}
	validation := normalized
	validation.Model = a.config.OAuthModelAliases.Resolve(string(normalized.Provider), normalized.Model)
	missing := missingRequestFields(validation)
	if len(missing) > 0 {
		return normalized, fmt.Errorf("missing %s: %w", strings.Join(missing, ","), ErrInvalidRequest)
	}
	return normalized, nil
}

func missingRequestFields(request Request) []string {
	var missing []string
	if request.AuthID == "" {
		missing = append(missing, "auth_id")
	}
	if request.Provider == "" || request.Provider == detector.ProviderUnknown {
		missing = append(missing, "provider")
	}
	if !noCycleManualAntigravity(request) {
		if strings.TrimSpace(request.CycleKey.String()) == "" {
			missing = append(missing, "cycle_key")
		}
		if request.Window == "" || request.Window == detector.WindowUnknown {
			missing = append(missing, "window")
		}
	}
	return missing
}

func noCycleManualAntigravity(request Request) bool {
	modelGroup, validModel := detector.InferModelGroup(request.Model)
	return request.Manual && request.Provider == detector.ProviderAntigravity &&
		validModel && request.ModelGroup != detector.ModelGroupNone && modelGroup == request.ModelGroup &&
		strings.TrimSpace(request.CycleKey.String()) == "" && request.ResetAt.IsZero() &&
		(request.Window == "" || request.Window == detector.WindowUnknown)
}

func (a *Activator) initialResult(request Request) Result {
	observedAt := request.ObservedAt
	if observedAt.IsZero() {
		observedAt = a.now().UTC()
	}
	validation := request
	validation.Model = a.config.OAuthModelAliases.Resolve(string(request.Provider), request.Model)
	return Result{
		AuthID:                   request.AuthID,
		Provider:                 string(request.Provider),
		Window:                   string(request.Window),
		CycleKey:                 request.CycleKey.String(),
		Status:                   StatusFailed,
		ObservedAt:               observedAt.UTC(),
		ResetAt:                  request.ResetAt.UTC(),
		Remaining:                request.Remaining,
		HasRemaining:             request.HasRemaining,
		noCycleManualAntigravity: noCycleManualAntigravity(validation),
	}
}
