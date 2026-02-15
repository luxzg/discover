package scheduler

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Runner interface {
	Run(context.Context) error
}

type Scheduler struct {
	dailyHHMM string
	runner    Runner
	mu        sync.Mutex
	running   bool
	state     RunState
}

func New(dailyHHMM string, runner Runner) *Scheduler {
	return &Scheduler{dailyHHMM: dailyHHMM, runner: runner}
}

type RunState struct {
	Running         bool      `json:"running"`
	CurrentSource   string    `json:"current_source"`
	StartedAt       time.Time `json:"started_at"`
	LastCompletedAt time.Time `json:"last_completed_at"`
	LastDurationMS  int64     `json:"last_duration_ms"`
	LastError       string    `json:"last_error"`
	LastSource      string    `json:"last_source"`
}

func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		for {
			next, err := nextRun(time.Now(), s.dailyHHMM)
			if err != nil {
				log.Printf("scheduler: invalid daily time %q: %v", s.dailyHHMM, err)
				return
			}
			wait := time.Until(next)
			if wait < 0 {
				wait = 0
			}
			log.Printf("scheduler: next ingestion at %s", next.Format(time.RFC3339))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			if err := s.run(ctx, "scheduled"); err != nil {
				log.Printf("scheduler: ingestion run error: %v", err)
			}
		}
	}()
}

func (s *Scheduler) RunNow(ctx context.Context) error {
	return s.run(ctx, "manual")
}

func (s *Scheduler) run(ctx context.Context, source string) error {
	const minRunGap = 15 * time.Second
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrIngestAlreadyRunning
	}
	if !s.state.LastCompletedAt.IsZero() {
		sinceLast := time.Since(s.state.LastCompletedAt)
		if sinceLast < minRunGap {
			s.mu.Unlock()
			return &runErr{msg: "ingestion just completed; wait a few seconds before starting again"}
		}
	}
	s.running = true
	s.state.Running = true
	s.state.CurrentSource = source
	s.state.StartedAt = time.Now()
	s.mu.Unlock()

	log.Printf("scheduler: ingestion started (source=%s)", source)
	start := time.Now()
	err := s.runner.Run(ctx)

	defer func() {
		s.mu.Lock()
		s.running = false
		s.state.Running = false
		s.state.CurrentSource = ""
		s.state.LastCompletedAt = time.Now()
		s.state.LastDurationMS = time.Since(start).Milliseconds()
		s.state.LastSource = source
		if err != nil {
			s.state.LastError = err.Error()
		} else {
			s.state.LastError = ""
		}
		s.mu.Unlock()
	}()
	if err != nil {
		log.Printf("scheduler: ingestion finished with error (source=%s, took=%s): %v", source, time.Since(start).Round(time.Millisecond), err)
		return err
	}
	log.Printf("scheduler: ingestion finished (source=%s, took=%s)", source, time.Since(start).Round(time.Millisecond))
	return nil
}

func (s *Scheduler) Snapshot() RunState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

var ErrIngestAlreadyRunning = &runErr{"ingestion already running"}

type runErr struct{ msg string }

func (e *runErr) Error() string { return e.msg }

func nextRun(now time.Time, hhmm string) (time.Time, error) {
	parts := strings.Split(hhmm, ":")
	if len(parts) != 2 {
		return time.Time{}, &runErr{"daily_ingest_time must be HH:MM"}
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return time.Time{}, &runErr{"invalid hour"}
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return time.Time{}, &runErr{"invalid minute"}
	}
	loc := now.Location()
	t := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc)
	if !t.After(now) {
		t = t.Add(24 * time.Hour)
	}
	return t, nil
}
