package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/user/miniweb/internal/archive"
	"github.com/user/miniweb/internal/auth"
	"github.com/user/miniweb/internal/browser"
	"github.com/user/miniweb/internal/compress"
	"github.com/user/miniweb/internal/config"
	imgpkg "github.com/user/miniweb/internal/image"
	"github.com/user/miniweb/internal/minidom"
	"github.com/user/miniweb/internal/minidom/mbpf"
	"github.com/user/miniweb/internal/minidom/text"
	"github.com/user/miniweb/internal/session"
)

type archiveHandler struct {
	store *archive.Store
	mgr   *session.Manager
	cfg   *config.Config
}

// create archives the current tab snapshot for offline reading.
// POST /api/v1/sessions/{sessID}/tabs/{tabID}/archive
func (h *archiveHandler) create(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	tabID := chi.URLParam(r, "tabID")
	userID := auth.UserIDFromContext(r.Context())

	sess, err := h.mgr.GetSession(sessID, userID)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	// Always archive as flow mode for compact, readable output.
	opts := browser.SnapshotOptions{
		Format:           "mbpf",
		RenderingProfile: "flow",
		ImageFormat:      h.cfg.Images.DefaultFormat,
		ImageQuality:     h.cfg.Images.DefaultQuality,
	}
	snap, err := h.mgr.Snapshot(sess, tabID, opts)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	// Fetch images inline so the archive is self-contained for offline use.
	inlineImages(snap, h.cfg)

	// Encode to MBPF and compress with brotli for compact storage.
	payload, err := mbpf.Encode(snap)
	if err != nil {
		writeError(w, "encode failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	compressed, err := compress.Compress(compress.AlgoBrotli, -1, payload)
	if err != nil {
		writeError(w, "compress failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	archiveID := strings.ReplaceAll(uuid.New().String(), "-", "")
	meta := archive.Meta{
		ID:         archiveID,
		UserID:     userID,
		URL:        snap.URL,
		Title:      snap.Title,
		FaviconURL: snap.FaviconURL,
		Format:     "mbpf+brotli",
		Size:       len(compressed),
		CreatedAt:  time.Now().UTC(),
	}

	maxPerUser := h.cfg.Archive.MaxPerUser
	if err := h.store.Save(meta, compressed, maxPerUser); err != nil {
		writeError(w, "save failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"archive_id": archiveID,
		"url":        snap.URL,
		"title":      snap.Title,
		"size":       len(compressed),
	})
}

// list returns all archives for the authenticated user.
// GET /api/v1/archives
func (h *archiveHandler) list(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	metas, err := h.store.List(userID)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type item struct {
		ID         string    `json:"id"`
		URL        string    `json:"url"`
		Title      string    `json:"title"`
		FaviconURL string    `json:"favicon_url,omitempty"`
		Size       int       `json:"size"`
		CreatedAt  time.Time `json:"created_at"`
	}
	result := make([]item, len(metas))
	for i, m := range metas {
		result[i] = item{
			ID:         m.ID,
			URL:        m.URL,
			Title:      m.Title,
			FaviconURL: m.FaviconURL,
			Size:       m.Size,
			CreatedAt:  m.CreatedAt,
		}
	}
	writeJSON(w, result)
}

// get serves an archived snapshot in the client's preferred format.
// GET /api/v1/archives/{archiveID}
func (h *archiveHandler) get(w http.ResponseWriter, r *http.Request) {
	archiveID := chi.URLParam(r, "archiveID")
	userID := auth.UserIDFromContext(r.Context())

	meta, data, err := h.store.Get(archiveID, userID)
	if err != nil {
		if err == archive.ErrNotFound {
			writeError(w, "not found", http.StatusNotFound)
		} else if err == archive.ErrForbidden {
			writeError(w, "forbidden", http.StatusForbidden)
		} else {
			writeError(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Decompress the stored brotli payload to get raw MBPF.
	mbpfData, err := compress.Decompress(compress.AlgoBrotli, data)
	if err != nil {
		writeError(w, "decompress failed", http.StatusInternalServerError)
		return
	}

	// Determine requested output format.
	accept := r.Header.Get("Accept")
	wantMBPF := strings.Contains(accept, "application/x-mbpf") || r.URL.Query().Get("format") == "mbpf"

	var payload []byte
	var contentType string

	if wantMBPF {
		payload = mbpfData
		contentType = "application/x-mbpf"
	} else {
		// Decode MBPF → PageSnapshot → re-encode as JSON.
		snap, decErr := mbpf.Decode(mbpfData)
		if decErr != nil {
			writeError(w, "decode failed: "+decErr.Error(), http.StatusInternalServerError)
			return
		}
		payload, err = text.EncodeJSON(snap)
		if err != nil {
			writeError(w, "encode failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		contentType = "application/minidom+json"
	}

	// Negotiate compression for transport.
	allowed := []string{compress.AlgoZstd, compress.AlgoGzip, compress.AlgoBrotli}
	algo := compress.Negotiate(r.Header.Get("Accept-Encoding"), allowed)
	if algo != compress.AlgoNone {
		payload, err = compress.Compress(algo, -1, payload)
		if err == nil {
			if ce := compress.ContentEncoding(algo); ce != "" {
				w.Header().Set("Content-Encoding", ce)
			}
		}
	}

	etag := fmt.Sprintf(`"%s"`, archiveID)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Type", contentType)
	if meta.FaviconURL != "" {
		w.Header().Set("X-Favicon-URL", meta.FaviconURL)
	}
	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}

// inlineImages fetches all ResourceRef images concurrently and stores them as
// InlineData so the archived snapshot is fully self-contained for offline use.
// Images that fail to fetch or exceed 2 MB are silently skipped.
func inlineImages(snap *minidom.PageSnapshot, cfg *config.Config) {
	if len(snap.Resources) == 0 {
		return
	}

	policy := imgpkg.FromSettings(
		cfg.Images.DefaultFormat,
		cfg.Images.DefaultQuality,
		cfg.Images.MaxWidth,
		cfg.Images.MaxHeight,
	)

	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := range snap.Resources {
		res := &snap.Resources[i]
		if res.URL == "" || len(res.InlineData) > 0 {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(r *minidom.ResourceRef) {
			defer wg.Done()
			defer func() { <-sem }()

			data, mime, err := imgpkg.Recompress(r.URL, policy)
			if err != nil {
				log.Printf("archive: skip image %s: %v", r.URL, err)
				return
			}
			if len(data) > 2<<20 { // skip > 2 MB
				return
			}
			mu.Lock()
			r.InlineData = data
			r.MIMEType = mime
			mu.Unlock()
		}(res)
	}
	wg.Wait()
}

// delete removes an archive entry.
// DELETE /api/v1/archives/{archiveID}
func (h *archiveHandler) delete(w http.ResponseWriter, r *http.Request) {
	archiveID := chi.URLParam(r, "archiveID")
	userID := auth.UserIDFromContext(r.Context())

	if err := h.store.Delete(archiveID, userID); err != nil {
		if err == archive.ErrNotFound {
			writeError(w, "not found", http.StatusNotFound)
		} else if err == archive.ErrForbidden {
			writeError(w, "forbidden", http.StatusForbidden)
		} else {
			writeError(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
