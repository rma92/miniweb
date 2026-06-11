package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/user/miniweb/internal/auth"
	"github.com/user/miniweb/internal/session"
)

type tabsHandler struct {
	mgr *session.Manager
}

func (h *tabsHandler) create(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	userID := auth.UserIDFromContext(r.Context())

	sess, err := h.mgr.GetSession(sessID, userID)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	var body struct {
		URL string `json:"url"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	tab, err := h.mgr.OpenTab(sess, body.URL)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"tab_id": tab.ID})
}

func (h *tabsHandler) close(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	tabID := chi.URLParam(r, "tabID")
	userID := auth.UserIDFromContext(r.Context())

	sess, err := h.mgr.GetSession(sessID, userID)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	if err := h.mgr.CloseTab(sess, tabID); err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *tabsHandler) navigate(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	tabID := chi.URLParam(r, "tabID")
	userID := auth.UserIDFromContext(r.Context())

	sess, err := h.mgr.GetSession(sessID, userID)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		writeError(w, "url required", http.StatusBadRequest)
		return
	}

	if err := h.mgr.Navigate(sess, tabID, body.URL); err != nil {
		writeBrowserError(w, err)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}
