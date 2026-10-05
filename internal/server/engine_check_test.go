package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"discover/internal/ingest"
)

type testEngineChecker struct{ entered, release chan struct{} }

func (p testEngineChecker) LastProgress() (string, time.Time) { return "", time.Time{} }
func (p testEngineChecker) LastProgressMessages(int) []string { return nil }
func (p testEngineChecker) CheckEngines(ctx context.Context) ingest.EngineCheckReport {
	close(p.entered)
	select {
	case <-p.release:
	case <-ctx.Done():
	}
	return ingest.EngineCheckReport{ConfiguredInstances: 1, CheckedInstances: 1, Categories: []ingest.CategoryCheck{{Instance: 1, Category: "news", Status: "empty"}}}
}

func TestEngineCheckAdminBoundaryAndServiceOwnedLifetime(t *testing.T) {
	a, h, user, ucsrf, admin, csrf := testAPI(t)
	checker := testEngineChecker{make(chan struct{}), make(chan struct{})}
	a.progress = checker
	if w := request(h, "POST", "/admin/api/search-check", "{}", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(h, "POST", "/admin/api/search-check", "{}", user, ucsrf); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(h, "POST", "/admin/api/search-check", "{}", admin, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("POST", "/admin/api/search-check", nil)
	r.RemoteAddr = "192.0.2.1:1"
	r.AddCookie(admin)
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r = httptest.NewRequest("POST", "/admin/api/search-check", nil).WithContext(ctx)
	r.RemoteAddr = "127.0.0.1:123"
	r.AddCookie(admin)
	r.Header.Set("X-CSRF-Token", csrf)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	cancel()
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body)
	}
	<-checker.entered
	if w := request(h, "POST", "/admin/api/ingest", "{}", admin, csrf); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := request(h, "POST", "/admin/api/search-check", "{}", admin, csrf); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := request(h, "GET", "/admin/api/search-check", "", user, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	close(checker.release)
	deadline := time.Now().Add(time.Second)
	for a.engineSnapshot().Status == "running" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if a.engineSnapshot().Status != "completed" {
		t.Fatal(a.engineSnapshot())
	}
	w = request(h, "GET", "/admin/api/status", "", admin, "")
	var result struct {
		SearchCheck engineCheckState `json:"search_check"`
	}
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.SearchCheck.Report.Categories[0].Status != "empty" {
		t.Fatal(w.Body)
	}
	for a.scheduler.Snapshot().SearchCheckRunning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if w := request(h, "POST", "/admin/api/search-check", "{}", admin, csrf); w.Code != 429 {
		t.Fatal(w.Code, w.Body)
	}
}
