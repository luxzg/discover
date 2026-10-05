package server

import (
	"errors"
	"net/http"
	"strconv"
)

func (a *API) handleAdminArchive(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		active, err := a.store.FeedAgeLimit(r.Context())
		if err != nil {
			respondErr(w, 500, err)
			return
		}
		days := active
		if raw := r.URL.Query().Get("days"); raw != "" {
			days, err = strconv.Atoi(raw)
			if err != nil {
				respondErr(w, 400, errors.New("age days must be an integer"))
				return
			}
		}
		if days < 0 || days > 36500 {
			respondErr(w, 400, errors.New("age days must be 0..36500"))
			return
		}
		stats, err := a.store.PreviewArchive(r.Context(), days)
		if err != nil {
			respondErr(w, 500, err)
			return
		}
		respondJSON(w, 200, map[string]any{"stats": stats, "active_days": active})
	case http.MethodPost:
		var req struct {
			Days *int `json:"days"`
		}
		if err := decodeJSON(r, a.cfg.MaxBodyBytes, &req); err != nil {
			respondErr(w, 400, err)
			return
		}
		if req.Days == nil || *req.Days < 0 || *req.Days > 36500 {
			respondErr(w, 400, errors.New("age days must be 0..36500"))
			return
		}
		stats, err := a.store.ArchiveOldUnread(r.Context(), *req.Days, true)
		if err != nil {
			respondErr(w, 500, err)
			return
		}
		respondJSON(w, 200, map[string]any{"stats": stats})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (a *API) handleAdminDomains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	items, err := a.store.DomainReport(r.Context())
	if err != nil {
		respondErr(w, 500, err)
		return
	}
	respondJSON(w, 200, map[string]any{"items": items})
}
