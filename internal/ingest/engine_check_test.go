package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"discover/internal/config"
)

func TestEngineCheckBoundedPacedAndRedacted(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("q") != "intel cpu" || r.URL.Query().Get("time_range") != "day" || r.URL.Query().Get("pageno") != "1" {
			t.Error(r.URL)
		}
		if r.URL.Query().Get("categories") == "news" {
			w.Write([]byte(`{"results":[{"engines":["brave.news"]}],"unresponsive_engines":[["startpage news","Suspended: CAPTCHA secret-fixture-token"]]}`))
		} else {
			w.Write([]byte(`{"results":[],"unresponsive_engines":[]}`))
		}
	}))
	defer upstream.Close()
	cfg := config.Config{}
	cfg.SearxngInstances = []string{upstream.URL, upstream.URL, upstream.URL, upstream.URL}
	cfg.SearchRequestDelaySeconds, cfg.SearchRequestJitterSeconds = 0, 0
	s := New(cfg, nil)
	waits := 0
	s.searchWait = func(ctx context.Context, delay time.Duration) error {
		waits++
		if delay < 5*time.Second {
			t.Error("diagnostic burst", delay)
		}
		return nil
	}
	report := s.CheckEngines(context.Background())
	if requests != 6 || waits != 5 || report.ConfiguredInstances != 4 || report.CheckedInstances != 3 || len(report.Categories) != 6 {
		t.Fatalf("%d %d %+v", requests, waits, report)
	}
	if report.Categories[0].Status != "warnings" || report.Categories[1].Status != "empty" {
		t.Fatal(report)
	}
	if report.Categories[0].Engines[0].Status != "returned_results" || report.Categories[0].Engines[1].Status != "captcha" {
		t.Fatal(report)
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "secret-fixture-token") || strings.Contains(string(encoded), upstream.URL) {
		t.Fatal("unsafe report", string(encoded))
	}
	if s.searchMinDelay != 0 {
		t.Fatal("diagnostic pacing leaked into ingestion")
	}
}

func TestEngineCheckHonorsCooldownAndCancellation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) }))
	defer upstream.Close()
	cfg := config.Config{}
	cfg.SearxngInstances = []string{upstream.URL}
	s := New(cfg, nil)
	report := s.CheckEngines(context.Background())
	if report.Categories[0].Code != "http_429" || report.Categories[1].Code != "instance_cooldown" {
		t.Fatal(report)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report = s.CheckEngines(ctx)
	if report.Error != "canceled" || len(report.Categories) != 0 {
		t.Fatal(report)
	}
}

func TestEngineCheckCancellationDuringPauseNeverSendsNextRequest(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.Write([]byte(`{"results":[]}`)) }))
	defer upstream.Close()
	s := New(config.Config{SearxngInstances: []string{upstream.URL}}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.searchWait = func(ctx context.Context, delay time.Duration) error { cancel(); return ctx.Err() }
	report := s.CheckEngines(ctx)
	if requests != 1 || report.Error != "canceled" || report.Categories[1].Code != "canceled" {
		t.Fatal(requests, report)
	}
}

func TestEngineObservationsAreBoundedAndWarningsWin(t *testing.T) {
	parsed := searxResponse{Results: []searxEntry{{Engines: []string{"google", "<script>"}}}, UnresponsiveEngines: []json.RawMessage{json.RawMessage(`["google","Suspended: access denied private response"]`), json.RawMessage(`{}`)}}
	rows, _ := observations(parsed)
	if rows[0].Name != "google" || rows[0].Status != "access_denied" || rows[1].Name != "unknown" {
		t.Fatal(rows)
	}
	for _, tc := range []struct{ message, code string }{{"too many requests", "rate_limited"}, {"timeout", "timeout"}, {"HTTP error", "http_error"}, {"Suspended", "suspended"}, {"private arbitrary text", "engine_error"}} {
		if engineWarning(tc.message) != tc.code {
			t.Fatal(tc)
		}
	}
	for i := 0; i < 100; i++ {
		parsed.Results = append(parsed.Results, searxEntry{Engines: []string{strings.Repeat("a", i%60+1)}})
	}
	rows, capped := observations(parsed)
	if len(rows) != 50 || !capped {
		t.Fatal(len(rows), capped)
	}
}
