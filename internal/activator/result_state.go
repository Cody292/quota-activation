package activator

import (
	"context"
	"errors"
	"fmt"

	"quota-activation/internal/state"
)

func (a *Activator) failAndStore(ctx context.Context, result Result, err error) (Result, error) {
	result.Status = failedStatus(result.Status)
	result.Success = false
	result.LastError = state.Redact(err.Error())
	stored, storeErr := a.storeResult(ctx, result)
	if storeErr != nil {
		return stored, errors.Join(err, storeErr)
	}
	return stored, err
}

func failedStatus(status Status) Status {
	if status == StatusSkipped || status == StatusBusy {
		return status
	}
	return StatusFailed
}

func (a *Activator) storeResult(ctx context.Context, result Result) (Result, error) {
	if a.store == nil {
		return result, ErrMissingDependency
	}
	a.store.Upsert(state.Record{
		AuthID:     result.AuthID,
		Provider:   result.Provider,
		Window:     result.Window,
		CycleKey:   result.CycleKey,
		ObservedAt: result.ObservedAt,
		ResetAt:    result.ResetAt,
		LastResult: string(result.Status),
		LastError:  result.LastError,
	})
	if err := a.store.SaveAtomic(ctx); err != nil {
		return result, fmt.Errorf("save activation state: %w", safeError(err))
	}
	return result, nil
}
