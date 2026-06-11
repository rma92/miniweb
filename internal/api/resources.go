package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/user/miniweb/internal/auth"
	"github.com/user/miniweb/internal/config"
	imgpkg "github.com/user/miniweb/internal/image"
	"github.com/user/miniweb/internal/session"
)

type resourcesHandler struct {
	mgr *session.Manager
	cfg *config.Config
}

func (h *resourcesHandler) get(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	tabID := chi.URLParam(r, "tabID")
	resourceID := chi.URLParam(r, "resourceID")
	userID := auth.UserIDFromContext(r.Context())

	sess, err := h.mgr.GetSession(sessID, userID)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	res, err := h.mgr.GetResource(sess, tabID, resourceID)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	// Resource IDs are stable per snapshot, so use the ID as an ETag.
	etag := `"` + resourceID + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Respond 304 if client already has this resource.
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Return cached inline data if available.
	if len(res.InlineData) > 0 {
		mimeType := res.MIMEType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", mimeType)
		w.Write(res.InlineData)
		return
	}

	if res.URL == "" {
		writeError(w, "resource has no URL", http.StatusNotFound)
		return
	}

	// Fetch and recompress on demand.
	policy := imgpkg.FromSettings(
		h.cfg.Images.DefaultFormat,
		h.cfg.Images.DefaultQuality,
		h.cfg.Images.MaxWidth,
		h.cfg.Images.MaxHeight,
	)

	data, mimeType, err := imgpkg.Recompress(res.URL, policy)
	if err != nil {
		writeError(w, "image fetch failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Cache result on the resource ref to avoid repeated re-fetch.
	res.InlineData = data
	res.MIMEType = mimeType

	w.Header().Set("Content-Type", mimeType)
	w.Write(data)
}
