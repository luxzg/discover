package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"sync"
	"time"
)

var errHideBusy = errors.New("another hide operation is running; wait for it to finish")
var errHideConflict = errors.New("request ID was already used for a different hide operation")

type hideJobState struct {
	ID         string  `json:"id"`
	Status     string  `json:"status"`
	Error      string  `json:"error,omitempty"`
	MatchedIDs []int64 `json:"matched_ids,omitempty"`
}
type hideJob struct {
	state     hideJobState
	signature string
}
type hideJobs struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	jobs   map[string]*hideJob
	order  []string
	active string
	closed bool
}

func newHideJobs() *hideJobs {
	ctx, cancel := context.WithCancel(context.Background())
	return &hideJobs{ctx: ctx, cancel: cancel, jobs: make(map[string]*hideJob)}
}
func copyHideState(s hideJobState) hideJobState {
	s.MatchedIDs = append([]int64(nil), s.MatchedIDs...)
	return s
}

// One active transaction, with a bounded completion cache for reconnects/retries.
func (j *hideJobs) start(id, signature string, run func(context.Context) ([]int64, error)) (hideJobState, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return hideJobState{}, errors.New("service is shutting down")
	}
	if old, ok := j.jobs[id]; ok {
		if old.signature != signature {
			return hideJobState{}, errHideConflict
		}
		return copyHideState(old.state), nil
	}
	if j.active != "" {
		return hideJobState{}, errHideBusy
	}
	if id == "" {
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			return hideJobState{}, err
		}
		id = hex.EncodeToString(token[:])
	}
	if len(j.order) >= 32 {
		delete(j.jobs, j.order[0])
		j.order = j.order[1:]
	}
	job := &hideJob{state: hideJobState{ID: id, Status: "running"}, signature: signature}
	j.jobs[id] = job
	j.order = append(j.order, id)
	j.active = id
	initial := copyHideState(job.state)
	j.wg.Add(1)
	go func() {
		defer j.wg.Done()
		ctx, cancel := context.WithTimeout(j.ctx, 10*time.Minute)
		defer cancel()
		start := time.Now()
		ids, err := run(ctx)
		j.mu.Lock()
		defer j.mu.Unlock()
		if err != nil {
			job.state.Status = "failed"
			job.state.Error = "Hide was not completed; check service logs and retry."
			log.Printf("hide: failed after %s: %v", time.Since(start).Round(time.Millisecond), err)
		} else {
			job.state.Status = "completed"
			job.state.MatchedIDs = ids
			log.Printf("hide: completed in %s", time.Since(start).Round(time.Millisecond))
		}
		j.active = ""
	}()
	return initial, nil
}
func (j *hideJobs) status(id string) (hideJobState, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if id == "" {
		id = j.active
	}
	job, ok := j.jobs[id]
	if !ok {
		return hideJobState{}, false
	}
	return copyHideState(job.state), true
}
func (j *hideJobs) shutdown() { j.mu.Lock(); j.closed = true; j.cancel(); j.mu.Unlock(); j.wg.Wait() }

// Shutdown cancels and waits for accepted mutations before the database closes.
func (a *API) Shutdown() { a.hideJobs.shutdown() }
