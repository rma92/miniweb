package api

import (
	"net/http"
	"runtime"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/user/miniweb/internal/config"
	"github.com/user/miniweb/internal/session"
)

type adminHandler struct {
	mgr *session.Manager
	cfg *config.Config
}

// sessions lists all active sessions across all users.
// GET /admin/sessions
func (h *adminHandler) sessions(w http.ResponseWriter, r *http.Request) {
	all := h.mgr.ListAllSessions()
	infos := make([]session.SessionInfo, 0, len(all))
	for _, s := range all {
		infos = append(infos, s.Info())
	}
	writeJSON(w, map[string]interface{}{
		"sessions": infos,
		"count":    len(infos),
	})
}

// deleteSession force-kills a session (bypasses ownership check).
// DELETE /admin/sessions/{sessionID}
func (h *adminHandler) deleteSession(w http.ResponseWriter, r *http.Request) {
	sessID := chi.URLParam(r, "sessionID")
	if err := h.mgr.ForceDeleteSession(sessID); err != nil {
		writeError(w, err.Error(), statusForSessionErr(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// configView returns the current configuration with sensitive fields redacted.
// GET /admin/config
func (h *adminHandler) configView(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg
	writeJSON(w, map[string]interface{}{
		"server": map[string]interface{}{
			"listen":        cfg.Server.ListenAddr,
			"https_enabled": cfg.Server.HTTPSEnabled,
		},
		"auth": map[string]interface{}{
			"enabled": cfg.Auth.Enabled,
		},
		"browser": map[string]interface{}{
			"engine":          cfg.Browser.Engine,
			"worker_pool_min": cfg.Browser.WorkerPoolMin,
			"worker_pool_max": cfg.Browser.WorkerPoolMax,
			"headless":        cfg.Browser.Headless,
		},
		"session": map[string]interface{}{
			"idle_timeout": cfg.Session.IdleTimeout.String(),
			"max_tabs":     cfg.Session.MaxTabs,
		},
		"adblock": map[string]interface{}{
			"enabled":               cfg.AdBlock.Enabled,
			"filter_list_count":     len(cfg.AdBlock.FilterListURLs),
			"filter_list_refresh_h": cfg.AdBlock.FilterListRefreshH,
		},
		"archive": map[string]interface{}{
			"enabled":      cfg.Archive.Enabled,
			"db_path":      cfg.Archive.DBPath,
			"max_per_user": cfg.Archive.MaxPerUser,
			"admin_token":  "[redacted]",
		},
	})
}

// status returns basic runtime stats.
// GET /admin/status
func (h *adminHandler) status(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	all := h.mgr.ListAllSessions()
	writeJSON(w, map[string]interface{}{
		"uptime_seconds": time.Since(startTime).Seconds(),
		"sessions":       len(all),
		"goroutines":     runtime.NumGoroutine(),
		"heap_mb":        float64(mem.HeapAlloc) / 1_000_000,
	})
}

var startTime = time.Now()

// adminAuthMiddleware checks the Authorization header against the admin token.
func adminAuthMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				writeError(w, "admin API disabled (no admin_token configured)", http.StatusForbidden)
				return
			}
			if r.Header.Get("Authorization") == "Bearer "+token {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="mininext-admin"`)
			writeError(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}
