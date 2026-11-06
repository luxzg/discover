package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"discover/internal/scheduler"
	"discover/internal/store"
)

type dedupeJobState struct {
	ID          string              `json:"id"`
	Status      string              `json:"status"`
	StartedAt   time.Time           `json:"started_at"`
	CompletedAt time.Time           `json:"completed_at"`
	Result      *store.DedupeResult `json:"result,omitempty"`
	Error       string              `json:"error,omitempty"`
}
type dedupeJob struct {
	state dedupeJobState
	done  chan struct{}
}
type dedupeJobs struct {
	mu     sync.Mutex
	jobs   map[string]*dedupeJob
	order  []string
	active string
	run    func(context.Context) (store.DedupeResult, error)
}

func newDedupeJobs(run func(context.Context) (store.DedupeResult, error)) *dedupeJobs {
	return &dedupeJobs{jobs: make(map[string]*dedupeJob), run: run}
}
func copyDedupeState(state dedupeJobState) dedupeJobState {
	if state.Result != nil {
		result := *state.Result
		state.Result = &result
	}
	return state
}
func (j *dedupeJobs) snapshot(id string) (dedupeJobState, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if id == "" {
		id = j.active
		if id == "" && len(j.order) > 0 {
			id = j.order[len(j.order)-1]
		}
		if id == "" {
			return dedupeJobState{Status: "idle"}, true
		}
	}
	job, ok := j.jobs[id]
	if !ok {
		return dedupeJobState{}, false
	}
	return copyDedupeState(job.state), true
}

// Publication and reservation share this lock, including instant completions.
// Simultaneous legacy/new callers join one job rather than starting two scans.
func (j *dedupeJobs) start(s *scheduler.Scheduler) (*dedupeJob, dedupeJobState, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.active != "" {
		job := j.jobs[j.active]
		return job, copyDedupeState(job.state), nil
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, dedupeJobState{}, err
	}
	id := hex.EncodeToString(token[:])
	job := &dedupeJob{state: dedupeJobState{ID: id, Status: "running", StartedAt: time.Now()}, done: make(chan struct{})}
	err := s.RequestDedupe(func(ctx context.Context) {
		result, err := j.run(ctx)
		j.mu.Lock()
		defer j.mu.Unlock()
		job.state.CompletedAt = time.Now()
		if err != nil {
			job.state.Status = "failed"
			job.state.Error = "Dedupe was not completed; check service logs before retrying."
			log.Printf("dedupe: failed after %s: %v", time.Since(job.state.StartedAt).Round(time.Millisecond), err)
		} else {
			job.state.Status = "completed"
			job.state.Result = &result
			log.Printf("dedupe: completed in %s (hidden=%d, total=%d)", time.Since(job.state.StartedAt).Round(time.Millisecond), result.Stats.SameRunHidden+result.Stats.HistoricalHidden, result.DedupeHiddenTotal)
		}
		j.active = ""
		close(job.done)
	})
	if err != nil {
		return nil, dedupeJobState{}, err
	}
	if len(j.order) >= 32 {
		delete(j.jobs, j.order[0])
		j.order = j.order[1:]
	}
	j.jobs[id] = job
	j.order = append(j.order, id)
	j.active = id
	return job, copyDedupeState(job.state), nil
}

func dedupeError(w http.ResponseWriter, err error) {
	code := http.StatusConflict
	if errors.Is(err, scheduler.ErrSchedulerStopped) {
		code = http.StatusServiceUnavailable
	}
	if !errors.Is(err, scheduler.ErrSchedulerStopped) && !errors.Is(err, scheduler.ErrIngestAlreadyRunning) && !errors.Is(err, scheduler.ErrSearchCheckRunning) && !errors.Is(err, scheduler.ErrDedupeRunning) {
		code = http.StatusInternalServerError
	}
	respondErr(w, code, err)
}

func (a *API) handleDedupeJobs(w http.ResponseWriter, r *http.Request) {
	var state dedupeJobState
	switch r.Method {
	case http.MethodGet:
		var ok bool
		state, ok = a.dedupeJobs.snapshot(r.URL.Query().Get("id"))
		if !ok {
			respondErr(w, http.StatusNotFound, errors.New("dedupe result unavailable; reload and verify database state"))
			return
		}
	case http.MethodPost:
		var err error
		_, state, err = a.dedupeJobs.start(a.scheduler)
		if err != nil {
			dedupeError(w, err)
			return
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code := http.StatusOK
	if r.Method == http.MethodPost {
		code = http.StatusAccepted
	}
	respondJSON(w, code, struct {
		dedupeJobState
		Schedule scheduler.RunState `json:"schedule"`
	}{state, a.scheduler.Snapshot()})
}
