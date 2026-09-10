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
