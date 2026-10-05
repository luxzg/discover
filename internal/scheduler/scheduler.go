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
	dailyHHMM  string
	interval   time.Duration
	runTimeout time.Duration
	runner     Runner
	mu         sync.Mutex
	running    bool
	state      RunState
	ctx        context.Context
	cancel     context.CancelFunc
	started    bool
	closed     bool
	wg         sync.WaitGroup
	checkDone  chan struct{}
}

func New(dailyHHMM string, intervalMinutes int, runner Runner) *Scheduler {
	var d time.Duration
	if intervalMinutes > 0 {
		d = time.Duration(intervalMinutes) * time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{dailyHHMM: dailyHHMM, interval: d, runTimeout: 2 * time.Hour, runner: runner, ctx: ctx, cancel: cancel}
}

type RunState struct {
	NextScheduledAt          *time.Time `json:"next_scheduled_at"`
	ScheduleMode             string     `json:"schedule_mode"`
	SearchCheckRunning       bool       `json:"search_check_running"`
	SearchCheckCooldownUntil time.Time  `json:"search_check_cooldown_until"`
	RunID                    uint64     `json:"run_id"`
	CooldownUntil            time.Time  `json:"cooldown_until"`
	Running                  bool       `json:"running"`
	CurrentSource            string     `json:"current_source"`
	StartedAt                time.Time  `json:"started_at"`
	LastCompletedAt          time.Time  `json:"last_completed_at"`
	LastDurationMS           int64      `json:"last_duration_ms"`
	LastError                string     `json:"last_error"`
	LastSource               string     `json:"last_source"`
}

func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started || s.closed {
		s.mu.Unlock()
		return
	}
	s.started = true
	if s.interval > 0 {
		next := time.Now().Add(s.interval)
		s.state.NextScheduledAt = &next
	} else if next, err := nextRun(time.Now(), s.dailyHHMM); err == nil {
		s.state.NextScheduledAt = &next
	}
	// Preserve cancellation for any already accepted manual job as well.
	stop := context.AfterFunc(ctx, s.cancel)
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer stop()
		if s.interval > 0 {
			s.startInterval(s.ctx)
			return
		}
		s.startDaily(s.ctx)
	}()
}

// Shutdown rejects new jobs, cancels active work and waits before the store closes.
func (s *Scheduler) Shutdown() {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Scheduler) startInterval(ctx context.Context) {
	initial := s.Snapshot().NextScheduledAt
	if initial == nil {
		return
	}
	next := *initial
	for {
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
		next = time.Now().Add(s.interval)
		s.setNext(next)
	}
}

func (s *Scheduler) startDaily(ctx context.Context) {
	for {
		next, err := nextRun(time.Now(), s.dailyHHMM)
		if err != nil {
			log.Printf("scheduler: invalid daily time %q: %v", s.dailyHHMM, err)
			return
		}
		wait := time.Until(next)
		s.setNext(next)
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
}

func (s *Scheduler) setNext(next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.NextScheduledAt = &next
}

// Checks share the ingestion exclusion gate but never change ingestion history.
// Accepted work belongs to the service, not the browser request.
func (s *Scheduler) RequestSearchCheck(check func(context.Context)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return ErrSchedulerStopped
	}
	if s.running {
		return ErrIngestAlreadyRunning
	}
	if s.state.SearchCheckRunning {
		return ErrSearchCheckRunning
	}
	if time.Now().Before(s.state.SearchCheckCooldownUntil) {
		return ErrSearchCheckCooldown
	}
	s.state.SearchCheckRunning = true
	s.state.SearchCheckCooldownUntil = time.Now().Add(5 * time.Minute)
	s.checkDone = make(chan struct{})
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			s.state.SearchCheckRunning = false
			close(s.checkDone)
			s.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(s.ctx, 3*time.Minute)
		defer cancel()
		check(ctx)
	}()
	return nil
}

func (s *Scheduler) RunNow(ctx context.Context) error {
	return s.run(ctx, "manual")
}

// RequestRun reserves a job synchronously and runs it independently of the request.
func (s *Scheduler) RequestRun() (RunState, error) {
	state, err := s.reserve("manual")
	if err != nil {
		return state, err
	}
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, s.runTimeout)
		defer cancel()
		_ = s.execute(ctx, "manual")
	}()
	return state, nil
}

func (s *Scheduler) reserve(source string) (RunState, error) {
	const minRunGap = 15 * time.Second
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return s.state, ErrSchedulerStopped
	}
	if s.running {
		return s.state, ErrIngestAlreadyRunning
	}
	if s.state.SearchCheckRunning {
		return s.state, ErrSearchCheckRunning
	}
	if !s.state.LastCompletedAt.IsZero() {
		sinceLast := time.Since(s.state.LastCompletedAt)
		if sinceLast < minRunGap {
			return s.state, ErrIngestCooldown
		}
	}
	s.running = true
	s.state.Running = true
	s.state.CurrentSource = source
	s.state.StartedAt = time.Now()
	s.state.RunID++
	s.state.LastError = ""
	if source == "scheduled" {
		s.state.NextScheduledAt = nil
	}
	s.wg.Add(1)
	return s.state, nil
}

func (s *Scheduler) run(ctx context.Context, source string) error {
	for {
		if _, err := s.reserve(source); err != nil {
			if source != "scheduled" || err != ErrSearchCheckRunning {
				return err
			}
			s.mu.Lock()
			done := s.checkDone
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
			}
			continue
		}
		break
	}
	runCtx, cancel := context.WithTimeout(ctx, s.runTimeout)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	return s.execute(runCtx, source)
}

func (s *Scheduler) execute(ctx context.Context, source string) error {
	defer s.wg.Done()
	log.Printf("scheduler: ingestion started (source=%s)", source)
	start := time.Now()
	err := s.runner.Run(ctx)

	defer func() {
		s.mu.Lock()
		s.running = false
		s.state.Running = false
		s.state.CurrentSource = ""
		s.state.LastCompletedAt = time.Now()
		s.state.CooldownUntil = s.state.LastCompletedAt.Add(15 * time.Second)
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
	state := s.state
	state.ScheduleMode = "daily"
	if s.interval > 0 {
		state.ScheduleMode = "interval"
	}
	if s.closed || s.ctx.Err() != nil {
		state.NextScheduledAt = nil
	}
	return state
}

var (
	ErrIngestAlreadyRunning = &runErr{"ingestion already running"}
	ErrIngestCooldown       = &runErr{"ingestion just completed; wait a few seconds before starting again"}
	ErrSchedulerStopped     = &runErr{"scheduler is shutting down"}
	ErrSearchCheckRunning   = &runErr{"search engine check is running; retry ingestion after it finishes"}
	ErrSearchCheckCooldown  = &runErr{"search engine check cooldown; wait five minutes between checks"}
)

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
		t = time.Date(now.Year(), now.Month(), now.Day()+1, h, m, 0, 0, loc)
	}
	return t, nil
}
