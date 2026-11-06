package server

import (
	"context"
	"discover/internal/store"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type observedDedupeWait struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (c *observedDedupeWait) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

func TestDedupeRejectsActiveIngestion(t *testing.T) {
	a, h, _, _, admin, csrf := testAPI(t)
	if _, err := a.scheduler.RequestRun(); err != nil {
		t.Fatal(err)
	}
	if w := request(h, "POST", "/admin/api/dedupe", "{}", admin, csrf); w.Code != 409 {
		t.Fatalf("dedupe overlapped ingestion: %d %s", w.Code, w.Body)
	}
}

func TestDedupeAsyncBoundaryLifetimeAndLegacyJoin(t *testing.T) {
	a, h, user, uc, admin, csrf := testAPI(t)
	for _, path := range []string{"/admin/api/dedupe", "/admin/api/dedupe/jobs"} {
		for _, tc := range []struct {
			cookie *http.Cookie
			csrf   string
			want   int
		}{{nil, "", 401}, {user, uc, 401}, {admin, "", 403}} {
			if w := request(h, "POST", path, "{}", tc.cookie, tc.csrf); w.Code != tc.want {
				t.Fatal(path, w.Code, w.Body)
			}
		}
	}
	r := httptest.NewRequest("POST", "/admin/api/dedupe/jobs", nil)
	r.RemoteAddr = "192.0.2.1:2"
	r.AddCookie(admin)
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	a.dedupeJobs.run = func(ctx context.Context) (store.DedupeResult, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return store.DedupeResult{}, ctx.Err()
		}
		return store.DedupeResult{DedupeHiddenTotal: 42, Stats: store.IngestDedupeStats{SameRunHidden: 2}}, nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	r = httptest.NewRequest("POST", "/admin/api/dedupe/jobs", nil).WithContext(ctx)
	r.RemoteAddr = "127.0.0.1:123"
	r.AddCookie(admin)
	r.Header.Set("X-CSRF-Token", csrf)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	cancel()
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body)
	}
	var accepted dedupeJobState
	if err := json.Unmarshal(w.Body.Bytes(), &accepted); err != nil || accepted.ID == "" || accepted.Status != "running" {
		t.Fatal(w.Body, err)
	}
	<-entered
	// Hold the sole database connection: status must remain memory-only.
	conn, err := a.store.DB().Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	statusDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { statusDone <- request(h, "GET", "/admin/api/dedupe/jobs?id="+accepted.ID, "", admin, "") }()
	select {
	case got := <-statusDone:
		if got.Code != 200 {
			t.Fatal(got.Code)
		}
	case <-time.After(time.Second):
		conn.Close()
		t.Fatal("status blocked on SQLite")
	}
	conn.Close()
	if got := request(h, "POST", "/api/feed/refresh", "{}", user, uc); got.Code != 409 {
		t.Fatal(got.Code)
	}
	if err := a.scheduler.RequestSearchCheck(func(context.Context) {}); err == nil {
		t.Fatal("engine check overlapped dedupe")
	}
	joined := request(h, "POST", "/admin/api/dedupe/jobs", "{}", admin, csrf)
	var second dedupeJobState
	json.Unmarshal(joined.Body.Bytes(), &second)
	if second.ID != accepted.ID {
		t.Fatal("did not join", joined.Body)
	}
	legacy := make(chan *httptest.ResponseRecorder, 1)
	legacyCtx := &observedDedupeWait{Context: t.Context(), entered: make(chan struct{})}
	legacyRequest := httptest.NewRequest("POST", "/admin/api/dedupe", nil).WithContext(legacyCtx)
	legacyRequest.RemoteAddr = "127.0.0.1:123"
	legacyRequest.AddCookie(admin)
	legacyRequest.Header.Set("X-CSRF-Token", csrf)
	go func() { reply := httptest.NewRecorder(); h.ServeHTTP(reply, legacyRequest); legacy <- reply }()
	<-legacyCtx.entered // The legacy handler has joined and reached its wait.
	// Cancel a legacy waiter, not the service-owned work.
	waitCtx, stop := context.WithCancel(t.Context())
	stop()
	r = httptest.NewRequest("POST", "/admin/api/dedupe", nil).WithContext(waitCtx)
	r.RemoteAddr = "127.0.0.1:123"
	r.AddCookie(admin)
	r.Header.Set("X-CSRF-Token", csrf)
	h.ServeHTTP(httptest.NewRecorder(), r)
	close(release)
	select {
	case got := <-legacy:
		if got.Code != 200 || !json.Valid(got.Body.Bytes()) {
			t.Fatal(got.Code, got.Body)
		}
		var result struct {
			OK bool `json:"ok"`
			store.DedupeResult
		}
		json.Unmarshal(got.Body.Bytes(), &result)
		if !result.OK || result.DedupeHiddenTotal != 42 || result.Stats.SameRunHidden != 2 {
			t.Fatal(got.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("legacy waiter did not finish")
	}
	state, _ := a.dedupeJobs.snapshot(accepted.ID)
	if state.Status != "completed" || state.Result == nil || state.Result.DedupeHiddenTotal != 42 {
		t.Fatal(state)
	}
	state.Result.DedupeHiddenTotal = 99
	again, _ := a.dedupeJobs.snapshot(accepted.ID)
	if again.Result.DedupeHiddenTotal != 42 {
		t.Fatal("snapshot alias")
	}
	if got := request(h, "GET", "/admin/api/dedupe/jobs?id=missing", "", admin, ""); got.Code != 404 {
		t.Fatal(got.Code)
	}
	if got := request(h, "GET", "/admin/api/dedupe/jobs", "", user, ""); got.Code != 401 {
		t.Fatal(got.Code)
	}
}

func TestDedupeShutdownAndBoundedResults(t *testing.T) {
	a, h, _, _, admin, csrf := testAPI(t)
	entered := make(chan struct{})
	a.dedupeJobs.run = func(ctx context.Context) (store.DedupeResult, error) {
		close(entered)
		<-ctx.Done()
		return store.DedupeResult{}, errors.New("synthetic private SQL detail")
	}
	w := request(h, "POST", "/admin/api/dedupe/jobs", "{}", admin, csrf)
	if w.Code != 202 {
		t.Fatal(w.Code)
	}
	<-entered
	a.Shutdown()
	state, _ := a.dedupeJobs.snapshot("")
	if state.Status != "failed" || state.Error == "synthetic private SQL detail" {
		t.Fatal(state)
	}
	if w := request(h, "POST", "/admin/api/dedupe/jobs", "{}", admin, csrf); w.Code != 503 {
		t.Fatal(w.Code)
	}
	b, _, _, _, _, _ := testAPI(t)
	var first string
	for i := 0; i < 35; i++ {
		job, state, err := b.dedupeJobs.start(b.scheduler)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = state.ID
		}
		<-job.done
		deadline := time.Now().Add(time.Second)
		for b.scheduler.Snapshot().DedupeRunning && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
	}
	if len(b.dedupeJobs.jobs) != 32 {
		t.Fatal(len(b.dedupeJobs.jobs))
	}
	if _, ok := b.dedupeJobs.snapshot(first); ok {
		t.Fatal("old result not evicted")
	}
}
