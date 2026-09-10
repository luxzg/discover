package server

import (
	"context"
	"discover/internal/store"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHideReturnsBeforeDatabaseWorkAndSurvivesRequestCancellation(t *testing.T) {
	a, h, user, csrf, admin, acsrf := testAPI(t)
	err := a.store.UpsertArticleHit(context.Background(), store.UpsertArticleInput{URL: "https://example.com/a", NormalizedURL: "https://example.com/a", URLHash: "fixture", Title: "A test story", SourceDomain: "example.com", IngestedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":1,"pattern":"example.com","penalty":100,"request_id":"retry-key","visible_ids":[1]}`
	if w := request(h, "POST", "/api/articles/dontshow", body, nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(h, "POST", "/api/articles/dontshow", body, admin, acsrf); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(h, "POST", "/api/articles/dontshow", body, user, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	lock, err := a.store.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest("POST", "/api/articles/dontshow", strings.NewReader(body)).WithContext(ctx)
	r.RemoteAddr = "127.0.0.1:123"
	r.AddCookie(user)
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	returned := make(chan struct{})
	go func() { h.ServeHTTP(w, r); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("hide blocked on database before accepting job")
	}
	cancel()
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body)
	}
	var state hideJobState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "running" {
		t.Fatal(state)
	}
	repeat := request(h, "POST", "/api/articles/dontshow", body, user, csrf)
	if repeat.Code != 202 {
		t.Fatal(repeat.Body)
	}
	if conflict := request(h, "POST", "/api/articles/dontshow", strings.Replace(body, "100", "99", 1), user, csrf); conflict.Code != 409 {
		t.Fatal(conflict.Code)
	}
	if guest := request(h, "GET", "/api/articles/dontshow/status?id="+state.ID, "", nil, ""); guest.Code != 401 {
		t.Fatal(guest.Code)
	}
	if running := request(h, "GET", "/api/articles/dontshow/status?id="+state.ID, "", user, ""); running.Code != 200 || !strings.Contains(running.Body.String(), "running") {
		t.Fatal(running.Body)
	}
	if err := lock.Rollback(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status := request(h, "GET", "/api/articles/dontshow/status?id="+state.ID, "", user, "")
		if err := json.Unmarshal(status.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
		if state.Status != "running" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if state.Status != "completed" || len(state.MatchedIDs) != 1 {
		t.Fatal(state)
	}
	var actual string
	if err := a.store.DB().QueryRow(`SELECT status FROM articles WHERE id=1`).Scan(&actual); err != nil || actual != "hidden" {
		t.Fatal(actual, err)
	}
}

func TestHideJobsBoundedAndShutdown(t *testing.T) {
	jobs := newHideJobs()
	entered := make(chan struct{})
	_, err := jobs.start("one", "one", func(ctx context.Context) ([]int64, error) { close(entered); <-ctx.Done(); return nil, ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if _, err := jobs.start("two", "two", func(context.Context) ([]int64, error) { return nil, nil }); err != errHideBusy {
		t.Fatal(err)
	}
	jobs.shutdown()
	state, _ := jobs.status("one")
	if state.Status != "failed" {
		t.Fatal(state)
	}
	if _, err := jobs.start("three", "three", nil); err == nil {
		t.Fatal("accepted after shutdown")
	}
}

func TestHideCompletionCacheIsBounded(t *testing.T) {
	jobs := newHideJobs()
	defer jobs.shutdown()
	for i := 0; i < 40; i++ {
		state, err := jobs.start("", "same", func(context.Context) ([]int64, error) { return []int64{1}, nil })
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(time.Second)
		for state.Status == "running" && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
			state, _ = jobs.status(state.ID)
		}
		if state.Status != "completed" {
			t.Fatal(state)
		}
	}
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if len(jobs.jobs) != 32 || len(jobs.order) != 32 {
		t.Fatal("unbounded completion cache")
	}
}
