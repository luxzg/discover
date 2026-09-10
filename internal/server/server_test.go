package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"discover/internal/auth"
	"discover/internal/config"
	"discover/internal/db"
	"discover/internal/model"
	"discover/internal/scheduler"
	"discover/internal/store"
)

type testRunner struct{}

func (testRunner) Run(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }

func testAPI(t *testing.T) (*API, http.Handler, *http.Cookie, string, *http.Cookie, string) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(d)
	if err := st.Prepare(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	g, _ := auth.New("test-admin", []string{"127.0.0.1/32"})
	u, _ := auth.NewUserGuard("reader", "test-reader")
	sched := scheduler.New("07:30", 120, testRunner{})
	a := New(config.Config{DefaultBatchSize: 10, FeedMinScore: 1, MaxBodyBytes: 4096, DedupeTitleKeyChars: 50, HideRuleDefaultPenalty: 100}, st, sched, nil, g, u, AssetsHandler())
	t.Cleanup(func() { sched.Shutdown(); a.Shutdown(); d.Close() })
	user, _, _ := u.NewSession("127.0.0.1:123", time.Hour)
	csrf, _ := u.SessionCSRF(user, "127.0.0.1:123")
	admin, _, _ := g.NewSession("127.0.0.1:123", time.Hour)
	acsrf, _ := g.SessionCSRF(admin, "127.0.0.1:123")
	return a, a.Routes(), &http.Cookie{Name: auth.UserSessionCookieName, Value: user}, csrf, &http.Cookie{Name: auth.SessionCookieName, Value: admin}, acsrf
}
func request(h http.Handler, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:123"
	if cookie != nil {
		r.AddCookie(cookie)
	}
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRefreshRequiresUserAndCSRFAndReturnsImmediately(t *testing.T) {
	_, h, user, csrf, admin, acsrf := testAPI(t)
	for _, tc := range []struct {
		cookie *http.Cookie
		csrf   string
		want   int
	}{{nil, "", 401}, {admin, acsrf, 401}, {user, "", 403}, {user, csrf, 202}} {
		w := request(h, "POST", "/api/feed/refresh", "{}", tc.cookie, tc.csrf)
		if w.Code != tc.want {
			t.Fatalf("%d: %s", w.Code, w.Body)
		}
	}
	if w := request(h, "POST", "/api/feed/refresh", "{}", user, csrf); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := request(h, "GET", "/api/feed/refresh/status", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w := request(h, "GET", "/api/feed/refresh/status", "", user, "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "last_error") {
		t.Fatal(w.Body)
	}
}

func TestRollingCookieAndAdminBoundary(t *testing.T) {
	_, h, user, _, admin, _ := testAPI(t)
	for _, tc := range []struct {
		path   string
		cookie *http.Cookie
		ttl    time.Duration
	}{{"/api/feed", user, 90 * 24 * time.Hour}, {"/admin/api/status", admin, 24 * time.Hour}} {
		w := request(h, "GET", tc.path, "", tc.cookie, "")
		if w.Code != 200 {
			t.Fatal(w.Body)
		}
		cookies := w.Result().Cookies()
		if len(cookies) == 0 || time.Until(cookies[0].Expires) < tc.ttl-time.Minute || !cookies[0].HttpOnly {
			t.Fatal("cookie not renewed")
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing cache policy")
		}
	}
	r := httptest.NewRequest("GET", "/admin/api/status", nil)
	r.RemoteAddr = "192.0.2.1:123"
	r.AddCookie(admin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}

func TestEditIDsAndJSONValidation(t *testing.T) {
	a, h, _, _, admin, csrf := testAPI(t)
	if err := a.store.UpsertTopic(context.Background(), model.Topic{Query: "wrong", Weight: 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	topics, _ := a.store.ListTopics(context.Background())
	b, _ := json.Marshal(model.Topic{ID: topics[0].ID, Query: "correct", Weight: 0, Enabled: false})
	w := request(h, "POST", "/admin/api/topics", string(b), admin, csrf)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	topics, _ = a.store.ListTopics(context.Background())
	if len(topics) != 1 || topics[0].Query != "correct" || topics[0].Weight != 0 {
		t.Fatal(topics)
	}
	for _, body := range []string{`{} {}`, strings.Repeat(" ", 4097), `{"unknown":1}`} {
		w := request(h, "POST", "/admin/api/topics", body, admin, csrf)
		if w.Code != 400 {
			t.Fatalf("got %d", w.Code)
		}
	}
}
