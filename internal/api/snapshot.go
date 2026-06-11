package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/user/miniweb/internal/auth"
	"github.com/user/miniweb/internal/browser"
	"github.com/user/miniweb/internal/compress"
	"github.com/user/miniweb/internal/config"
	"github.com/user/miniweb/internal/metrics"
	"github.com/user/miniweb/internal/minidom"
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
	if q := r.URL.Query().Get("format"); q != "" {
		format = q
	}

	// Rendering profile: "box" (default, full DOM) or "flow" (linearized).
	renderingProfile := "box"
	if q := r.URL.Query().Get("rendering"); q == "flow" || q == "box" {
		renderingProfile = q
	}

	opts := browser.SnapshotOptions{
		Format:           format,
		RenderingProfile: renderingProfile,
		ImageFormat:      h.cfg.Images.DefaultFormat,
		ImageQuality:     h.cfg.Images.DefaultQuality,
	}

	// If client advertises a base snapshot ID, try to serve a delta.
	sinceID := 0
	if s := r.URL.Query().Get("since"); s != "" {
		sinceID, _ = strconv.Atoi(s)
	}

	// Fetch the base snapshot (for delta) before taking the new snapshot.
	var baseSnap *minidom.PageSnapshot
	if sinceID > 0 {
		baseSnap = h.mgr.GetLastSnap(sess, tabID, sinceID)
	}

	snap, err := h.mgr.Snapshot(sess, tabID, opts)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	// Try delta if client has the right base snapshot.
	if baseSnap != nil {
		if delta := minidom.ComputeDelta(baseSnap, snap); delta != nil {
			payload, encErr := json.Marshal(delta)
			if encErr == nil {
				metrics.SnapshotBytes.WithLabelValues("delta", renderingProfile).Observe(float64(len(payload)))
				metrics.DeltaSnapshotsSent.Inc()
				allowed := []string{compress.AlgoZstd, compress.AlgoGzip, compress.AlgoBrotli}
				algo := compress.Negotiate(r.Header.Get("Accept-Encoding"), allowed)
				if algo != compress.AlgoNone {
					payload, encErr = compress.Compress(algo, -1, payload)
					if encErr == nil {
						if ce := compress.ContentEncoding(algo); ce != "" {
							w.Header().Set("Content-Encoding", ce)
						}
						metrics.SnapshotCompressedBytes.WithLabelValues("delta", algo).Observe(float64(len(payload)))
					}
				}
				w.Header().Set("Content-Type", "application/minidom-delta+json")
				w.Header().Set("X-Snapshot-Id", fmt.Sprintf("%d", snap.SnapshotID))
				w.WriteHeader(http.StatusOK)
				w.Write(payload)
				return
			}
		}
	}

	// Encode full snapshot.
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

	uncompressedSize := len(payload)
	metrics.SnapshotBytes.WithLabelValues(format, renderingProfile).Observe(float64(uncompressedSize))
	metrics.FullSnapshotsSent.Inc()

	// Offer all supported compression algorithms; client's Accept-Encoding decides.
	allowed := []string{compress.AlgoZstd, compress.AlgoGzip, compress.AlgoBrotli}
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
		metrics.SnapshotCompressedBytes.WithLabelValues(format, algo).Observe(float64(len(payload)))
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Snapshot-Id", fmt.Sprintf("%d", snap.SnapshotID))
	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}
