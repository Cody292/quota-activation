package management

import (
	"context"
	"errors"
	"net/http"

	"quota-activation/internal/activator"
	"quota-activation/internal/host"
	"quota-activation/internal/state"
)

// ServeHTTP 分发管理 API 请求和插件资源页请求。
func (h *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	switch {
	case request.Method == http.MethodGet && request.URL.Path == resourceStatusPath:
		h.handlePage(w)
	case request.Method == http.MethodGet && (request.URL.Path == managementPrefix || request.URL.Path == managementPrefix+"/status"):
		h.handleStatus(w)
	case request.Method == http.MethodGet && request.URL.Path == managementPrefix+"/auth-files":
		h.handleAuthFiles(w, request)
	case request.Method == http.MethodPost && request.URL.Path == managementPrefix+"/activate":
		h.handleActivate(w, request)
	case request.Method == http.MethodGet && request.URL.Path == managementPrefix+"/diagnostics":
		h.handleDiagnostics(w)
	default:
		writeError(w, http.StatusNotFound, "not_found", "not found")
	}
}

func (h *Handler) handleActivate(w http.ResponseWriter, request *http.Request) {
	if request.Context().Err() != nil {
		writeError(w, http.StatusRequestTimeout, "request_canceled", "request canceled")
		return
	}
	decoded, err := decodeActivationRequest(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	activation, err := decoded.toActivatorRequest(h.config, h.now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := h.activator.Activate(request.Context(), activation)
	if h.onActivation != nil {
		h.onActivation(result, err)
	}
	response := responseFromResult(result)
	if err != nil {
		response.LastError = state.Redact(err.Error())
		h.storeLatest(response)
		writeJSON(w, statusForActivationError(err), response)
		return
	}
	h.storeLatest(response)
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) handleStatus(w http.ResponseWriter) {
	response := h.latestResponse()
	response.LatestStatus = latestStatus(response.Status)
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) handleDiagnostics(w http.ResponseWriter) {
	response := h.latestResponse()
	response.LatestStatus = latestStatus(response.Status)
	response.Routes = Register().Routes
	response.Resources = Register().Resources
	response.Config = &diagnosticConfig{
		AutoActivate:           h.config.AutoActivate,
		EnableBeforeActivation: h.config.EnableBeforeActivation,
		ActivationModelCodex:   state.Redact(h.config.ActivationModels.Codex.Models),
	}
	if response.AuthID != "" {
		response.StateRecords = []state.Record{{
			AuthID:     response.AuthID,
			Provider:   response.Provider,
			Window:     response.Window,
			CycleKey:   response.CycleKey,
			ObservedAt: response.ObservedAt,
			ResetAt:    response.ResetAt,
			LastResult: response.Status,
			LastError:  response.LastError,
		}}
	}
	if h.runHistory != nil {
		if history := h.runHistory(); history != nil {
			response.RunHistory = history
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) storeLatest(response activationResponse) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.latest = response
}

func (h *Handler) latestResponse() activationResponse {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.latest
}

func latestStatus(status string) string {
	if status == "" {
		return "idle"
	}
	return state.Redact(status)
}

func statusForActivationError(err error) int {
	switch {
	case errors.Is(err, activator.ErrBusy), errors.Is(err, activator.ErrDisabledCredential):
		return http.StatusConflict
	case errors.Is(err, host.ErrModelExecuteStatus):
		return http.StatusBadGateway
	case errors.Is(err, activator.ErrNetworkFailure), activator.IsNetworkFailure(err):
		return http.StatusBadGateway
	case errors.Is(err, activator.ErrInvalidRequest):
		return http.StatusBadRequest
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return http.StatusRequestTimeout
	default:
		return http.StatusInternalServerError
	}
}
