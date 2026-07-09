package activator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"quota-activation/internal/host"
	"quota-activation/internal/scheduler"
	"quota-activation/internal/session"
)

func (a *Activator) execute(ctx context.Context, request Request, result Result) (Result, error) {
	callCtx := ctx
	cancel := func() {}
	if a.config.ActivationRequestTimeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, a.config.ActivationRequestTimeout)
	}
	defer cancel()
	created, err := a.sessions.Create(session.Target{AuthID: request.AuthID, Provider: string(request.Provider), Window: string(request.Window)})
	if err != nil {
		return result, fmt.Errorf("create activation session: %w", safeError(err))
	}
	body, err := json.Marshal(modelPing{Model: request.Model, Prompt: request.Prompt})
	if err != nil {
		return result, fmt.Errorf("encode activation ping: %w", safeError(err))
	}
	response, err := a.host.ModelExecute(callCtx, host.ModelExecuteRequest{
		Model:   request.Model,
		Headers: map[string][]string{scheduler.NonceHeaderName: {created.Nonce}},
		Body:    body,
	})
	result.Nonce = created.Nonce
	if err != nil {
		result.HTTPStatus = statusCodeFromError(err)
		return result, fmt.Errorf("host model execute: %w", safeHostError(err))
	}
	checked, err := host.ResponseOrStatusError(response)
	if err != nil {
		result.HTTPStatus = response.StatusCode
		return result, fmt.Errorf("host model execute: %w", safeHostError(err))
	}
	result.HTTPStatus = checked.StatusCode
	return result, nil
}

type modelPing struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

func statusCodeFromError(err error) int {
	var statusErr *host.HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode
	}
	return 0
}
