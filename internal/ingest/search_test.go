package ingest

import (
	"context"
	"discover/internal/config"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHarvestMatrixAndEmptySuccess(t *testing.T) {
	seen := map[string]bool{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		key := q.Get("categories") + ":" + q.Get("time_range") + ":" + q.Get("pageno")
		if seen[key] {
			t.Errorf("duplicate request %s", key)
		}
		seen[key] = true
		if q.Get("q") != "intel gpu" || q.Has("count") || q.Has("category_images") {
			t.Errorf("query %v", q)
		}
		fmt.Fprint(w, `{"results":[]}`)
	}))
	defer ts.Close()
	s := New(config.Config{SearxngInstances: []string{ts.URL + "?category_images=1&count=100"}}, nil)
	results, err := s.fetchTopic(context.Background(), "intel+gpu")
	if err != nil || len(results) != 0 || len(seen) != 8 {
		t.Fatalf("%d %d %v", len(results), len(seen), err)
	}
	for _, c := range []string{"news", "general"} {
		for _, r := range []string{"day", "week"} {
			for _, p := range []int{1, 2} {
				if !seen[fmt.Sprintf("%s:%s:%d", c, r, p)] {
					t.Fatal("matrix incomplete")
				}
			}
		}
	}
}

func TestSearchRedirectCannotReachOtherOrigin(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true; fmt.Fprint(w, `{"results":[]}`) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer source.Close()
	s := New(config.Config{}, nil)
	if _, _, err := s.fetchFromInstance(context.Background(), source.URL, "topic", "general", "week", 1); err == nil {
		t.Fatal("cross-origin redirect accepted")
	}
	if reached {
		t.Fatal("redirect destination reached")
	}
}

func TestSearchRejectsSameOriginRedirect(t *testing.T) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Redirect(w, r, "/other", http.StatusFound)
	}))
	defer ts.Close()
	s := New(config.Config{}, nil)
	if _, _, err := s.fetchFromInstance(context.Background(), ts.URL, "topic", "general", "week", 1); err == nil {
		t.Fatal("redirect accepted")
	}
	if requests != 1 {
		t.Fatal("redirect followed", requests)
	}
}

func TestPartialAndMalformedResults(t *testing.T) {
	for _, body := range []string{`{}`, `{"results":null}`, `<html>`, strings.Repeat("x", (4<<20)+1)} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		s := New(config.Config{}, nil)
		if _, _, err := s.fetchFromInstance(context.Background(), ts.URL, "x", "news", "day", 1); err == nil {
			t.Fatal("accepted invalid response")
		}
		ts.Close()
	}
	n := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 2 {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, `{"results":[{"url":"https://example.com/a","title":"One story"}]}`)
	}))
	defer ts.Close()
	s := New(config.Config{SearxngInstances: []string{ts.URL}}, nil)
	results, err := s.fetchTopic(context.Background(), "topic")
	if err == nil || len(results) != 1 {
		t.Fatalf("partial results: %v %v", results, err)
	}
	if _, blocked := s.blockRemaining(ts.URL); !blocked {
		t.Fatal("missing cooldown")
	}
}

func TestMergeAndPublished(t *testing.T) {
	out := []searxEntry{}
	seen := map[string]int{}
	mergeEntries(&out, seen, []searxEntry{{URL: "https://example.com/a", Title: "Story", Engines: []string{"one"}}})
	mergeEntries(&out, seen, []searxEntry{{URL: "https://example.com/a?utm_source=test", Title: "Story", Thumbnail: "https://example.com/img", PublishedDate: "2026-09-01T10:00:00Z", Engines: []string{"two"}}})
	if len(out) != 1 || out[0].Thumbnail == "" || len(out[0].Engines) != 2 || out[0].PublishedDate == "" {
		t.Fatal(out)
	}
	if !parsePublished("", "").IsZero() {
		t.Fatal("fabricated publication")
	}
	want := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	if got := parsePublished("Thu, 10 Sep 2026 10:00:00 EST", ""); !got.Equal(want) {
		t.Fatal(got)
	}
	if got := parsePublished("bad", "2026-09-10"); got.IsZero() {
		t.Fatal("no fallback")
	}
}

func TestHTMLMetadata(t *testing.T) {
	base, _ := url.Parse("https://example.com/redirected/article")
	for _, tc := range []struct{ body, want string }{
		{`<meta property=og:image content="../cover?a=1&amp;b=2">`, "https://example.com/cover?a=1&b=2"},
		{`<base href="https://cdn.example/assets/"><link rel=image_src href=cover.jpg>`, "https://cdn.example/assets/cover.jpg"},
		{`<meta property=og:image content="javascript:alert(1)"><meta name=twitter:image content=/good>`, "https://example.com/good"},
		{`<meta property=og:image content=/one content=/two>`, "https://example.com/one"},
	} {
		if got := thumbnailFromHTML(base, []byte(tc.body)); got != tc.want {
			t.Fatalf("%s != %s", got, tc.want)
		}
	}
}

func TestBoundedRedactedFailures(t *testing.T) {
	p := &PartialRunError{}
	for i := 0; i < 100; i++ {
		p.add(Failure{Stage: "fetch", Code: "operation_failed", cause: errors.New("secret-token")})
	}
	p.add(dbFailure("fetch", context.Canceled))
	merged := &PartialRunError{}
	merged.merge(p, 1, 2)
	if merged.Total != 101 || len(merged.Failures) != maxFailureDetails || !errors.Is(merged, context.Canceled) || strings.Contains(merged.Error(), "secret-token") {
		t.Fatalf("%+v", merged)
	}
}
