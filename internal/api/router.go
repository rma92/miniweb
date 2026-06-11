package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/user/miniweb/internal/archive"
	"github.com/user/miniweb/internal/auth"
	"github.com/user/miniweb/internal/config"
	"github.com/user/miniweb/internal/metrics"
	"github.com/user/miniweb/internal/session"
)

// NewRouter builds and returns the chi router for the full REST API.
// The webFS handler is used to serve the static HTML5 client.
func NewRouter(mgr *session.Manager, cfg *config.Config, tokenStore auth.Store, archiveStore *archive.Store, webHandler http.Handler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(metrics.Middleware)

	// Prometheus metrics endpoint (unauthenticated by design — same as most infra).
	r.Handle("/metrics", metrics.Handler())

	// Apply auth middleware to API routes only.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.Middleware(tokenStore, cfg.Auth.Enabled))

		sh := &sessionsHandler{mgr: mgr}
		r.Post("/sessions", sh.create)
		r.Delete("/sessions/{sessionID}", sh.delete)
		r.Post("/sessions/{sessionID}/sleep", sh.sleep)
		r.Post("/sessions/{sessionID}/resume", sh.resume)
		r.Post("/sessions/{sessionID}/adblock", sh.adblock)

		th := &tabsHandler{mgr: mgr}
		r.Post("/sessions/{sessionID}/tabs", th.create)
		r.Post("/sessions/{sessionID}/tabs/{tabID}/navigate", th.navigate)

		snapH := &snapshotHandler{mgr: mgr, cfg: cfg}
		r.Get("/sessions/{sessionID}/tabs/{tabID}/snapshot", snapH.get)

		intH := &interactHandler{mgr: mgr}
		r.Post("/sessions/{sessionID}/tabs/{tabID}/interact", intH.post)

		resH := &resourcesHandler{mgr: mgr, cfg: cfg}
		r.Get("/sessions/{sessionID}/tabs/{tabID}/resources/{resourceID}", resH.get)

		// Archive routes (enabled unconditionally; Store may be nil if disabled).
		if archiveStore != nil {
			archH := &archiveHandler{store: archiveStore, mgr: mgr, cfg: cfg}
			r.Post("/sessions/{sessionID}/tabs/{tabID}/archive", archH.create)
			r.Get("/archives", archH.list)
			r.Get("/archives/{archiveID}", archH.get)
			r.Delete("/archives/{archiveID}", archH.delete)
		}
	})

	// Admin API — protected by a separate admin token.
	adminH := &adminHandler{mgr: mgr, cfg: cfg}
	r.Route("/admin", func(r chi.Router) {
		r.Use(adminAuthMiddleware(cfg.Archive.AdminToken))
		r.Get("/sessions", adminH.sessions)
		r.Delete("/sessions/{sessionID}", adminH.deleteSession)
		r.Get("/config", adminH.configView)
		r.Get("/status", adminH.status)
	})

	// Serve static web client for everything else.
	r.Handle("/*", webHandler)

	return r
}
