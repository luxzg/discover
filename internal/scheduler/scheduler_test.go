package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

type blockingRunner struct {
	entered chan struct{}
	exited  chan struct{}
}

func (b blockingRunner) Run(ctx context.Context) error {
	close(b.entered)
	<-ctx.Done()
	close(b.exited)
	return ctx.Err()
}
func TestAsyncLifecycle(t *testing.T) {
	b := blockingRunner{make(chan struct{}), make(chan struct{})}
	s := New("07:30", 120, b)
	state, err := s.RequestRun()
	if err != nil || !state.Running || state.RunID != 1 {
		t.Fatalf("%+v %v", state, err)
	}
	<-b.entered
	if _, err := s.RequestRun(); !errors.Is(err, ErrIngestAlreadyRunning) {
		t.Fatal(err)
	}
	s.Shutdown()
	select {
	case <-b.exited:
	default:
		t.Fatal("shutdown did not wait")
	}
	if _, err := s.RequestRun(); !errors.Is(err, ErrSchedulerStopped) {
		t.Fatal(err)
	}
	if s.Snapshot().Running {
		t.Fatal("still running")
	}
}

func TestPacedRunDeadlinesAndCancellation(t *testing.T) {
	for _, async := range []bool{false, true} {
		b := blockingRunner{make(chan struct{}), make(chan struct{})}
		s := New("07:30", 120, b)
		if s.runTimeout != 2*time.Hour {
			t.Fatal("paced ingestion deadline too short", s.runTimeout)
		}
		s.runTimeout = 10 * time.Millisecond
		if async {
			if _, err := s.RequestRun(); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := s.RunNow(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
		}
		select {
		case <-b.exited:
		case <-time.After(time.Second):
			t.Fatal("run ignored safety deadline")
		}
		s.Shutdown()
		if s.Snapshot().Running {
			t.Fatal("deadline left run reserved")
		}
	}
}

func TestDailyDST(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Zagreb")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, loc)
	next, err := nextRun(now, "07:30")
	if err != nil {
		t.Fatal(err)
	}
	if next.Day() != 29 || next.Hour() != 7 || next.Minute() != 30 {
		t.Fatal(next)
	}
	if next.Sub(now) != 18*time.Hour+30*time.Minute {
		t.Fatal(next.Sub(now))
	}
}

func TestVisibleScheduleAndCheckExclusion(t *testing.T) {
	b := blockingRunner{make(chan struct{}), make(chan struct{})}
	s := New("07:30", 120, b)
	s.Start(context.Background())
	t.Cleanup(s.Shutdown)
	before := s.Snapshot()
	if before.NextScheduledAt == nil || time.Until(*before.NextScheduledAt) < 119*time.Minute || before.ScheduleMode != "interval" {
		t.Fatal(before)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	if err := s.RequestSearchCheck(func(ctx context.Context) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
		}
	}); err != nil {
		t.Fatal(err)
	}
	<-entered
	if _, err := s.RequestRun(); err != ErrSearchCheckRunning {
		t.Fatal(err)
	}
	if err := s.RequestSearchCheck(func(context.Context) {}); err != ErrSearchCheckRunning {
		t.Fatal(err)
	}
	s.mu.Lock()
	done := s.checkDone
	s.mu.Unlock()
	close(release)
	<-done
	if err := s.RequestSearchCheck(func(context.Context) {}); err != ErrSearchCheckCooldown {
		t.Fatal(err)
	}
	if s.Snapshot().RunID != 0 || !s.Snapshot().NextScheduledAt.Equal(*before.NextScheduledAt) {
		t.Fatal("diagnostic changed ingestion history")
	}
	if _, err := s.RequestRun(); err != nil {
		t.Fatal(err)
	}
	<-b.entered
	if err := s.RequestSearchCheck(func(context.Context) {}); err != ErrIngestAlreadyRunning {
		t.Fatal(err)
	}
	if !s.Snapshot().NextScheduledAt.Equal(*before.NextScheduledAt) {
		t.Fatal("manual run changed schedule")
	}
	s.Shutdown()
	if s.Snapshot().NextScheduledAt != nil {
		t.Fatal("stopped scheduler advertises run")
	}
}

func TestScheduledRunWaitsForCheckAndShutdownCancels(t *testing.T) {
	b := blockingRunner{make(chan struct{}), make(chan struct{})}
	s := New("07:30", 120, b)
	t.Cleanup(s.Shutdown)
	entered, release := make(chan struct{}), make(chan struct{})
	if err := s.RequestSearchCheck(func(ctx context.Context) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
		}
	}); err != nil {
		t.Fatal(err)
	}
	<-entered
	go func() { _ = s.run(s.ctx, "scheduled") }()
	select {
	case <-b.entered:
		t.Fatal("scheduled ingestion overlaps check")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	select {
	case <-b.entered:
	case <-time.After(time.Second):
		t.Fatal("due ingestion was skipped")
	}
	s.Shutdown()
	if err := s.RequestSearchCheck(func(context.Context) {}); err != ErrSchedulerStopped {
		t.Fatal(err)
	}

	s2 := New("07:30", 0, b)
	canceled := make(chan struct{})
	if err := s2.RequestSearchCheck(func(ctx context.Context) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 3*time.Minute {
			t.Error("unbounded search check")
		}
		<-ctx.Done()
		close(canceled)
	}); err != nil {
		t.Fatal(err)
	}
	s2.Shutdown()
	select {
	case <-canceled:
	default:
		t.Fatal("check survives shutdown")
	}
}

func TestInitialDailySchedule(t *testing.T) {
	s := New("07:30", 0, blockingRunner{make(chan struct{}), make(chan struct{})})
	s.Start(context.Background())
	defer s.Shutdown()
	state := s.Snapshot()
	if state.NextScheduledAt == nil || state.ScheduleMode != "daily" || state.NextScheduledAt.Hour() != 7 || state.NextScheduledAt.Minute() != 30 || !state.NextScheduledAt.After(time.Now()) {
		t.Fatal(state)
	}
}
