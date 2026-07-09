package runtime

import (
	"encoding/json"
	"fmt"

	"quota-activation/internal/scheduler"
)

// SchedulerResponse 是 scheduler.pick 返回给 CPA 的保守调度决策。
type SchedulerResponse struct {
	Handled bool   `json:"Handled"`
	AuthID  string `json:"AuthID,omitempty"`
	Reason  string `json:"Reason"`
}

func (r *Runtime) pickSchedule(raw []byte) []byte {
	picker, err := r.schedulerPicker()
	if err != nil {
		return failure(err)
	}
	request, err := decodeSchedulerRequest(raw)
	if err != nil {
		return failure(err)
	}
	decision := picker.Pick(request)
	return envelopeResult(SchedulerResponse{Handled: decision.Handled, AuthID: decision.AuthID, Reason: decision.Reason}, nil)
}

func (r *Runtime) schedulerPicker() (scheduler.Picker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shutdown {
		return scheduler.Picker{}, ErrShutdown
	}
	return r.picker, nil
}

func decodeSchedulerRequest(raw []byte) (scheduler.PickRequest, error) {
	if len(raw) == 0 {
		return scheduler.PickRequest{}, nil
	}
	var wire schedulerRequestWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return scheduler.PickRequest{}, fmt.Errorf("%w: decode scheduler request: %v", ErrInvalidRequest, err)
	}
	return wire.toPickRequest(), nil
}

type schedulerRequestWire struct {
	Candidates []schedulerCandidateWire `json:"Candidates"`
	Headers    map[string][]string      `json:"Headers"`
	Metadata   map[string][]string      `json:"Metadata"`
}

type schedulerCandidateWire struct {
	AuthID         string `json:"AuthID"`
	AuthIDLower    string `json:"auth_id"`
	AuthIndex      string `json:"AuthIndex"`
	AuthIndexLower string `json:"auth_index"`
}

func (w schedulerRequestWire) toPickRequest() scheduler.PickRequest {
	candidates := make([]scheduler.Candidate, 0, len(w.Candidates))
	for _, candidate := range w.Candidates {
		candidates = append(candidates, scheduler.Candidate{AuthID: firstNonEmpty(candidate.AuthID, candidate.AuthIDLower, candidate.AuthIndex, candidate.AuthIndexLower)})
	}
	return scheduler.PickRequest{Candidates: candidates, Headers: w.Headers, Metadata: w.Metadata}
}
