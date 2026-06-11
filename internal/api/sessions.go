package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/user/miniweb/internal/auth"
	"github.com/user/miniweb/internal/browser"
	"github.com/user/miniweb/internal/session"
)

type sessionsHandler struct {
	mgr *session.Manager
}

type createSessionRequest struct {
	DeviceProfile string `json:"device_profile"`
	Capabilities  struct {
		PageFormats       []string `json:"page_formats"`
		Compression       []string `json:"compression"`
		ImageFormats      []string `json:"image_formats"`
		RenderingProfiles []string `json:"rendering_profiles"`
		AdBlock           *bool    `json:"adblock"` // nil = use server default
	} `json:"capabilities"`
}

type createSessionResponse struct {
	SessionID      string          `json:"session_id"`
	ExpiresSeconds int             `json:"expires_in_seconds"`
	SelectedProfile selectedProfile `json:"selected_profile"`
}

type selectedProfile struct {
	PageFormat       string `json:"page_format"`
	Compression      string `json:"compression"`
	ImageFormat      string `json:"image_format"`
	ImageQuality     string `json:"image_quality"`
	RenderingProfile string `json:"rendering_profile"`
}

func (h *sessionsHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Default to phone-modern if no body.
		req.DeviceProfile = "phone-modern"
	}

	profileName := req.DeviceProfile
	if profileName == "" {
		profileName = "phone-modern"
	}

	profile, ok := browser.DefaultProfiles[profileName]
	if !ok {
		profile = browser.DefaultProfiles["phone-modern"]
	}

	userID := auth.UserIDFromContext(r.Context())
	sess, err := h.mgr.CreateSession(userID, profile)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If the client explicitly specifies an adblock preference, apply it now.
	// nil means "use server default" (already set by Worker.CreateSession).
	if req.Capabilities.AdBlock != nil {
		_ = h.mgr.SetAdBlock(sess, *req.Capabilities.AdBlock)
	}

	writeJSON(w, createSessionResponse{
		SessionID:      sess.ID,
		ExpiresSeconds: 600,
		SelectedProfile: selectedProfile{
			PageFormat:       "minidom-text",
			Compression:      "gzip",
			ImageFormat:      "jpeg",
			ImageQuality:     "medium",
			RenderingProfile: "box",
		},
	})
}

func (h *sessionsHandler) delete(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	userID := auth.UserIDFromContext(r.Context())

	if err := h.mgr.DeleteSession(sessID, userID); err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *sessionsHandler) sleep(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	userID := auth.UserIDFromContext(r.Context())

	if err := h.mgr.SleepSession(sessID, userID); err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}
	writeJSON(w, map[string]string{"status": "sleeping"})
}

func (h *sessionsHandler) resume(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	userID := auth.UserIDFromContext(r.Context())

	if err := h.mgr.ResumeSession(sessID, userID); err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}
	writeJSON(w, map[string]string{"status": "active"})
}

func (h *sessionsHandler) adblock(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	userID := auth.UserIDFromContext(r.Context())

	sess, err := h.mgr.GetSession(sessID, userID)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.mgr.SetAdBlock(sess, body.Enabled); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"adblock_enabled": body.Enabled})
}
