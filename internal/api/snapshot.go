package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/user/miniweb/internal/auth"
	"github.com/user/miniweb/internal/browser"
	"github.com/user/miniweb/internal/compress"
	"github.com/user/miniweb/internal/config"
	"github.com/user/miniweb/internal/minidom/mbpf"
	"github.com/user/miniweb/internal/minidom/text"
	"github.com/user/miniweb/internal/session"
)

type snapshotHandler struct {
	mgr *session.Manager
	cfg *config.Config
}

func (h *snapshotHandler) get(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	tabID := chi.URLParam(r, "tabID")
	userID := auth.UserIDFromContext(r.Context())

	sess, err := h.mgr.GetSession(sessID, userID)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	// Determine requested format.
	accept := r.Header.Get("Accept")
	format := h.cfg.Encoding.DefaultPageFormat
	if strings.Contains(accept, "application/x-mbpf") {
		format = "mbpf"
	} else if strings.Contains(accept, "application/minidom+json") || h.cfg.Encoding.AllowMinidomText {
		format = "minidom-text"
	}
	// Query param override for easy debugging.
	if q := r.URL.Query().Get("format"); q != "" {
		format = q
	}

	opts := browser.SnapshotOptions{
		Format:           format,
		RenderingProfile: "box",
		ImageFormat:      h.cfg.Images.DefaultFormat,
		ImageQuality:     h.cfg.Images.DefaultQuality,
	}

	snap, err := h.mgr.Snapshot(sess, tabID, opts)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	// Encode.
	var payload []byte
	var contentType string
	switch format {
	case "mbpf":
		payload, err = mbpf.Encode(snap)
		contentType = "application/x-mbpf"
	default:
		payload, err = text.EncodeJSON(snap)
		contentType = "application/minidom+json"
	}
	if err != nil {
		writeError(w, "encoding failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Negotiate and apply compression.
	allowed := []string{compress.AlgoGzip}
	if h.cfg.Encoding.DefaultCompression == "brotli" || strings.Contains(h.cfg.Encoding.DefaultCompression, "brotli") {
		allowed = append(allowed, compress.AlgoBrotli)
	}
	algo := compress.Negotiate(r.Header.Get("Accept-Encoding"), allowed)

	if algo != compress.AlgoNone {
		payload, err = compress.Compress(algo, -1, payload)
		if err != nil {
			writeError(w, "compression failed", http.StatusInternalServerError)
			return
		}
		if ce := compress.ContentEncoding(algo); ce != "" {
			w.Header().Set("Content-Encoding", ce)
		}
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Snapshot-Id", fmt.Sprintf("%d", snap.SnapshotID))
	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}
