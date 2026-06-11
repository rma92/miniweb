package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/user/miniweb/internal/auth"
	"github.com/user/miniweb/internal/session"
)

type eventsHandler struct {
	mgr *session.Manager
}

// stream upgrades the connection to a Server-Sent Events stream for the given
// tab. The client receives "ready" events when async navigation completes and
// "error" events when it fails. A heartbeat comment is sent every 25 seconds
// to keep proxies and load-balancers from closing idle connections.
func (h *eventsHandler) stream(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	tabID := chi.URLParam(r, "tabID")
	userID := auth.UserIDFromContext(r.Context())

	sess, err := h.mgr.GetSession(sessID, userID)
	if err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}

	tab, ok := h.mgr.GetTab(sess, tabID)
	if !ok {
		writeError(w, "tab not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx: disable proxy buffering
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	events, unsub := tab.Subscribe()
	defer unsub()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return
			}
			b, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()

		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
			sess.Touch() // prevent idle expiry while SSE is connected

		case <-r.Context().Done():
			return
		}
	}
}
