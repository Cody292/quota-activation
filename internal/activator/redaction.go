package activator

import (
	"context"
	"errors"

	"quota-activation/internal/host"
	"quota-activation/internal/state"
)

type redactedError struct {
	message string
	cause   error
}

func (e redactedError) Error() string { return e.message }
func (e redactedError) Unwrap() error { return e.cause }

func safeError(err error) error {
	if err == nil {
		return nil
	}
	var statusErr *host.HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr
	}
	message := state.Redact(err.Error())
	if message == "" {
		message = "activation error"
	}
	return redactedError{message: message, cause: err}
}

func safeHostError(err error) error {
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
	return redactedError{message: "host model execute failed", cause: err}
}
