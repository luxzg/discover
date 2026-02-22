package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"discover/internal/auth"
	"discover/internal/config"
	"discover/internal/model"
	"discover/internal/scheduler"
	"discover/internal/store"
)

type API struct {
	cfg       config.Config
	store     *store.Store
	scheduler *scheduler.Scheduler
	progress  progressSource
	guard     *auth.Guard
	user      *auth.UserGuard
	assets    http.Handler
}

type progressSource interface {
	LastProgress() (string, time.Time)
}

func New(cfg config.Config, st *store.Store, sched *scheduler.Scheduler, progress progressSource, guard *auth.Guard, user *auth.UserGuard, assets http.Handler) *API {
	return &API{cfg: cfg, store: st, scheduler: sched, progress: progress, guard: guard, user: user, assets: assets}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/assets/", a.assets)
	mux.HandleFunc("/", a.serveFeedUI)
	mux.HandleFunc("/admin", a.serveAdminUI)

	mux.Handle("/api/login", a.withJSON(http.HandlerFunc(a.handleUserLogin)))
	mux.Handle("/api/logout", a.userOnly(a.userCSRF(a.withJSON(http.HandlerFunc(a.handleUserLogout)))))
	mux.Handle("/api/session", a.withJSON(http.HandlerFunc(a.handleUserSession)))
	mux.Handle("/api/feed", a.userOnly(a.withJSON(http.HandlerFunc(a.handleFeed))))
	mux.Handle("/api/feed/seen", a.userOnly(a.userCSRF(a.withJSON(http.HandlerFunc(a.handleMarkSeen)))))
	mux.Handle("/api/feed/refresh", a.userOnly(a.userCSRF(a.withJSON(http.HandlerFunc(a.handleFeedRefresh)))))
	mux.Handle("/api/articles/action", a.userOnly(a.userCSRF(a.withJSON(http.HandlerFunc(a.handleArticleAction)))))
	mux.Handle("/api/articles/click", a.userOnly(a.userCSRF(a.withJSON(http.HandlerFunc(a.handleArticleClick)))))
	mux.Handle("/api/articles/dontshow", a.userOnly(a.userCSRF(a.withJSON(http.HandlerFunc(a.handleDontShow)))))

	mux.Handle("/admin/api/login", a.withJSON(http.HandlerFunc(a.handleAdminLogin)))
	mux.Handle("/admin/api/logout", a.guard.AdminOnly(a.adminCSRF(a.withJSON(http.HandlerFunc(a.handleAdminLogout)))))
	mux.Handle("/admin/api/session", a.guard.AdminOnly(a.withJSON(http.HandlerFunc(a.handleAdminSession))))
	mux.Handle("/admin/api/topics", a.guard.AdminOnly(a.adminCSRF(a.withJSON(http.HandlerFunc(a.handleAdminTopics)))))
	mux.Handle("/admin/api/rules", a.guard.AdminOnly(a.adminCSRF(a.withJSON(http.HandlerFunc(a.handleAdminRules)))))
	mux.Handle("/admin/api/ingest", a.guard.AdminOnly(a.adminCSRF(a.withJSON(http.HandlerFunc(a.handleAdminIngest)))))
	mux.Handle("/admin/api/dedupe", a.guard.AdminOnly(a.adminCSRF(a.withJSON(http.HandlerFunc(a.handleAdminDedupe)))))
	mux.Handle("/admin/api/status", a.guard.AdminOnly(a.withJSON(http.HandlerFunc(a.handleAdminStatus))))
	return mux
}

func (a *API) serveFeedUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, WebFS, "web/index.html")
}

func (a *API) serveAdminUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" {
		http.NotFound(w, r)
		return
	}
	if !a.guard.AllowRemote(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFileFS(w, r, WebFS, "web/admin.html")
}

func (a *API) handleFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := a.cfg.DefaultBatchSize
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	items, err := a.store.FetchTopUnread(r.Context(), limit, a.cfg.FeedMinScore)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) handleMarkSeen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSON(r, a.cfg.MaxBodyBytes, &req); err != nil {
		respondErr(w, http.StatusBadRequest, err)
		return
	}
	if err := a.store.MarkIDsAsSeen(r.Context(), req.IDs); err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) handleFeedRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := a.scheduler.RunNow(ctx); err != nil {
		if errors.Is(err, scheduler.ErrIngestAlreadyRunning) || errors.Is(err, scheduler.ErrIngestCooldown) {
			respondErr(w, http.StatusConflict, err)
			return
		}
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) handleArticleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID     int64  `json:"id"`
		Action string `json:"action"`
	}
	if err := decodeJSON(r, a.cfg.MaxBodyBytes, &req); err != nil {
		respondErr(w, http.StatusBadRequest, err)
		return
	}
	var err error
	switch req.Action {
	case "up":
		err = a.store.MarkIDStatus(r.Context(), req.ID, model.StatusUseful, 1.0)
	case "down", "hide":
		err = a.store.MarkIDStatus(r.Context(), req.ID, model.StatusHidden, -2.5)
	default:
		respondErr(w, http.StatusBadRequest, errors.New("invalid action"))
		return
	}
	if err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) handleArticleClick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := decodeJSON(r, a.cfg.MaxBodyBytes, &req); err != nil {
		respondErr(w, http.StatusBadRequest, err)
		return
	}
	if err := a.store.MarkRead(r.Context(), req.ID); err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) handleDontShow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID      int64   `json:"id"`
		Pattern string  `json:"pattern"`
		Penalty float64 `json:"penalty"`
	}
	if err := decodeJSON(r, a.cfg.MaxBodyBytes, &req); err != nil {
		respondErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Penalty <= 0 {
		req.Penalty = 10
	}
	if err := a.store.UpsertNegativeRule(r.Context(), model.NegativeRule{Pattern: req.Pattern, Penalty: req.Penalty, Enabled: true}); err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := a.store.MarkIDStatus(r.Context(), req.ID, model.StatusHidden, -req.Penalty); err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) handleAdminTopics(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		topics, err := a.store.ListTopics(r.Context())
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err)
			return
		}
		stats, err := a.store.TopicStats(r.Context())
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"items": topics, "topic_stats": stats})
	case http.MethodPost:
		var req model.Topic
		if err := decodeJSON(r, a.cfg.MaxBodyBytes, &req); err != nil {
			respondErr(w, http.StatusBadRequest, err)
			return
		}
		if req.Weight == 0 {
			req.Weight = 1
		}
		if err := a.store.UpsertTopic(r.Context(), req); err != nil {
			respondErr(w, http.StatusInternalServerError, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, err)
			return
		}
		if err := a.store.DeleteTopic(r.Context(), id); err != nil {
			respondErr(w, http.StatusInternalServerError, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleAdminRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules, err := a.store.ListNegativeRules(r.Context())
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"items": rules})
	case http.MethodPost:
		var req model.NegativeRule
		if err := decodeJSON(r, a.cfg.MaxBodyBytes, &req); err != nil {
			respondErr(w, http.StatusBadRequest, err)
			return
		}
		if req.Penalty == 0 {
			req.Penalty = 5
		}
		if err := a.store.UpsertNegativeRule(r.Context(), req); err != nil {
			respondErr(w, http.StatusInternalServerError, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, err)
			return
		}
		if err := a.store.DeleteNegativeRule(r.Context(), id); err != nil {
			respondErr(w, http.StatusInternalServerError, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleAdminIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := a.scheduler.RunNow(ctx); err != nil {
		if errors.Is(err, scheduler.ErrIngestAlreadyRunning) || errors.Is(err, scheduler.ErrIngestCooldown) {
			respondErr(w, http.StatusConflict, err)
			return
		}
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) handleAdminDedupe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	stats, err := a.store.HideAllUnreadTitleDuplicates(ctx, a.cfg.DedupeTitleKeyChars)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	total, err := a.store.DedupeHiddenTotal(r.Context())
	if err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"ok":                  true,
		"stats":               stats,
		"dedupe_hidden_total": total,
	})
}

func (a *API) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.guard.AllowRemote(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Secret string `json:"secret"`
	}
	if err := decodeJSON(r, a.cfg.MaxBodyBytes, &req); err != nil {
		respondErr(w, http.StatusBadRequest, err)
		return
	}
	if err := a.guard.ValidateSecret(req.Secret, r.RemoteAddr); err != nil {
		if errors.Is(err, auth.ErrBlocked) {
			respondErr(w, http.StatusTooManyRequests, err)
			return
		}
		respondErr(w, http.StatusUnauthorized, err)
		return
	}
	token, expires, err := a.guard.NewSession(r.RemoteAddr, 24*time.Hour)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	csrfToken, ok := a.guard.SessionCSRF(token, r.RemoteAddr)
	if !ok {
		respondErr(w, http.StatusInternalServerError, errors.New("session initialization failed"))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   a.cfg.EnableTLS,
		Expires:  expires,
	})
	respondJSON(w, http.StatusOK, map[string]any{
		"ok":                        true,
		"csrf_token":                csrfToken,
		"hide_rule_default_penalty": a.cfg.HideRuleDefaultPenalty,
	})
}

func (a *API) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if token, err := r.Cookie(auth.SessionCookieName); err == nil {
		a.guard.DeleteSession(token.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   a.cfg.EnableTLS,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	counts, err := a.store.ArticleStatusCounts(r.Context())
	if err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	dedupeHiddenTotal, err := a.store.DedupeHiddenTotal(r.Context())
	if err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	msg, msgAt := "", time.Time{}
	if a.progress != nil {
		msg, msgAt = a.progress.LastProgress()
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"ingest": map[string]any{
			"state":           a.scheduler.Snapshot(),
			"last_message":    msg,
			"last_message_at": msgAt,
		},
		"counts":              counts,
		"dedupe_hidden_total": dedupeHiddenTotal,
	})
}

func (a *API) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c, err := r.Cookie(auth.SessionCookieName)
	if err != nil || !a.guard.ValidateSession(c.Value, r.RemoteAddr) {
		respondErr(w, http.StatusUnauthorized, errors.New("sign in required"))
		return
	}
	csrfToken, ok := a.guard.SessionCSRF(c.Value, r.RemoteAddr)
	if !ok {
		respondErr(w, http.StatusUnauthorized, errors.New("sign in required"))
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"ok":                        true,
		"csrf_token":                csrfToken,
		"hide_rule_default_penalty": a.cfg.HideRuleDefaultPenalty,
	})
}

func (a *API) handleUserLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Secret   string `json:"secret"`
	}
	if err := decodeJSON(r, a.cfg.MaxBodyBytes, &req); err != nil {
		respondErr(w, http.StatusBadRequest, err)
		return
	}
	if err := a.user.ValidateCredentials(req.Username, req.Secret, r.RemoteAddr); err != nil {
		if errors.Is(err, auth.ErrUserBlocked) {
			respondErr(w, http.StatusTooManyRequests, err)
			return
		}
		respondErr(w, http.StatusUnauthorized, err)
		return
	}
	token, expires, err := a.user.NewSession(r.RemoteAddr, 30*24*time.Hour)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, err)
		return
	}
	csrfToken, ok := a.user.SessionCSRF(token, r.RemoteAddr)
	if !ok {
		respondErr(w, http.StatusInternalServerError, errors.New("session initialization failed"))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.UserSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   a.cfg.EnableTLS,
		Expires:  expires,
	})
	respondJSON(w, http.StatusOK, map[string]any{
		"ok":                        true,
		"csrf_token":                csrfToken,
		"hide_rule_default_penalty": a.cfg.HideRuleDefaultPenalty,
	})
}

func (a *API) handleUserLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if token, err := r.Cookie(auth.UserSessionCookieName); err == nil {
		a.user.DeleteSession(token.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.UserSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   a.cfg.EnableTLS,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) handleUserSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c, err := r.Cookie(auth.UserSessionCookieName)
	if err != nil || !a.user.ValidSession(c.Value, r.RemoteAddr) {
		respondErr(w, http.StatusUnauthorized, errors.New("sign in required"))
		return
	}
	csrfToken, ok := a.user.SessionCSRF(c.Value, r.RemoteAddr)
	if !ok {
		respondErr(w, http.StatusUnauthorized, errors.New("sign in required"))
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"ok":                        true,
		"csrf_token":                csrfToken,
		"hide_rule_default_penalty": a.cfg.HideRuleDefaultPenalty,
	})
}

func decodeJSON(r *http.Request, maxBody int64, out any) error {
	if maxBody <= 0 {
		maxBody = 1 << 20
	}
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func (a *API) withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

func (a *API) userOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(auth.UserSessionCookieName)
		if err != nil || !a.user.ValidSession(c.Value, r.RemoteAddr) {
			respondErr(w, http.StatusUnauthorized, errors.New("sign in required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) userCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(auth.UserSessionCookieName)
		if err != nil || !a.user.ValidateCSRF(c.Value, r.RemoteAddr, r.Header.Get("X-CSRF-Token")) {
			respondErr(w, http.StatusForbidden, errors.New("csrf token invalid"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) adminCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(auth.SessionCookieName)
		if err != nil || !a.guard.ValidateCSRF(c.Value, r.RemoteAddr, r.Header.Get("X-CSRF-Token")) {
			respondErr(w, http.StatusForbidden, errors.New("csrf token invalid"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func respondJSON(w http.ResponseWriter, code int, payload any) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondErr(w http.ResponseWriter, code int, err error) {
	respondJSON(w, code, map[string]any{"error": err.Error()})
}
