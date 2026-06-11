package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/user/miniweb/internal/auth"
	"github.com/user/miniweb/internal/browser"
	"github.com/user/miniweb/internal/session"
)

type interactHandler struct {
	mgr *session.Manager
}

type interactRequest struct {
	SnapshotID       int    `json:"snapshot_id"`
	RenderingProfile string `json:"rendering_profile"`
	Event            struct {
		Type       string `json:"type"`
		ElementID  int    `json:"element_id"`
		Value      string `json:"value"`
		ScrollX    int    `json:"scroll_x"`
		ScrollY    int    `json:"scroll_y"`
		FormValues []struct {
			ElementID int    `json:"element_id"`
			Value     string `json:"value"`
		} `json:"form_values"`
	} `json:"event"`
}

func (h *interactHandler) post(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	tabID := chi.URLParam(r, "tabID")
	userID := auth.UserIDFromContext(r.Context())

	sess, err := h.mgr.GetSession(sessID, userID)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	var req interactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	formVals := make([]browser.FormValue, 0, len(req.Event.FormValues))
	for _, fv := range req.Event.FormValues {
		formVals = append(formVals, browser.FormValue{ElementID: fv.ElementID, Value: fv.Value})
	}

	event := browser.InteractionEvent{
		Type:       req.Event.Type,
		ElementID:  req.Event.ElementID,
		Value:      req.Event.Value,
		ScrollX:    req.Event.ScrollX,
		ScrollY:    req.Event.ScrollY,
		FormValues: formVals,
	}

	if err := h.mgr.Interact(sess, tabID, event); err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	// Resolve rendering profile: body > query param > default "box".
	renderingProfile := req.RenderingProfile
	if renderingProfile == "" {
		if q := r.URL.Query().Get("rendering"); q == "flow" || q == "box" {
			renderingProfile = q
		}
	}
	if renderingProfile != "flow" {
		renderingProfile = "box"
	}

	snap, err := h.mgr.Snapshot(sess, tabID, browser.SnapshotOptions{
		Format:           "minidom-text",
		RenderingProfile: renderingProfile,
	})
	if err != nil {
		// Interaction succeeded but snapshot failed; return ok with no new snap.
		writeJSON(w, map[string]interface{}{"ok": true})
		return
	}

	writeJSON(w, map[string]interface{}{
		"ok":          true,
		"snapshot_id": snap.SnapshotID,
		"url":         snap.URL,
		"title":       snap.Title,
		"snapshot":    snap,
	})
}
