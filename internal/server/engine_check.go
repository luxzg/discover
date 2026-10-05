package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"discover/internal/ingest"
	"discover/internal/scheduler"
)

type engineChecker interface {
	CheckEngines(context.Context) ingest.EngineCheckReport
}

type engineCheckState struct {
	ID          uint64                   `json:"id"`
	Status      string                   `json:"status"`
	StartedAt   time.Time                `json:"started_at"`
	CompletedAt time.Time                `json:"completed_at"`
	Report      ingest.EngineCheckReport `json:"report"`
}

func (a *API) engineSnapshot() engineCheckState {
	a.engineMu.Lock()
	defer a.engineMu.Unlock()
	state := a.engineState
	if state.Status == "" {
		state.Status = "idle"
	}
	return state
}

func (a *API) handleEngineCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		respondJSON(w, http.StatusOK, a.engineSnapshot())
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	checker, ok := a.progress.(engineChecker)
	if !ok {
		respondErr(w, http.StatusServiceUnavailable, errors.New("search checker unavailable"))
		return
	}
	// Reserve and publish acceptance under one lock, including instant callbacks.
	a.engineMu.Lock()
	err := a.scheduler.RequestSearchCheck(func(ctx context.Context) {
		report := checker.CheckEngines(ctx)
		a.engineMu.Lock()
		defer a.engineMu.Unlock()
		a.engineState.Report = report
		a.engineState.CompletedAt = time.Now()
		a.engineState.Status = "completed"
		if report.Error != "" {
			a.engineState.Status = "failed"
		}
	})
	if err != nil {
		a.engineMu.Unlock()
		code := http.StatusConflict
		if errors.Is(err, scheduler.ErrSearchCheckCooldown) {
			code = http.StatusTooManyRequests
		}
		if errors.Is(err, scheduler.ErrSchedulerStopped) {
			code = http.StatusServiceUnavailable
		}
		respondErr(w, code, err)
		return
	}
	a.engineState = engineCheckState{ID: a.engineState.ID + 1, Status: "running", StartedAt: time.Now()}
	state := a.engineState
	a.engineMu.Unlock()
	respondJSON(w, http.StatusAccepted, state)
}
