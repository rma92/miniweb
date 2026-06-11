package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/miniweb/internal/adblock"
	"github.com/user/miniweb/internal/api"
	"github.com/user/miniweb/internal/archive"
	"github.com/user/miniweb/internal/auth"
	"github.com/user/miniweb/internal/browser"
	cdpworker "github.com/user/miniweb/internal/browser/chromedp"
	"github.com/user/miniweb/internal/config"
	"github.com/user/miniweb/internal/session"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	cfg := config.Load()

	// Browser worker pool: spawn cfg.Browser.WorkerPoolMin workers, up to WorkerPoolMax.
	poolSize := cfg.Browser.WorkerPoolMin
	if poolSize < 1 {
		poolSize = 1
	}
	if cfg.Browser.WorkerPoolMax > poolSize {
		poolSize = cfg.Browser.WorkerPoolMax
	}
	poolWorkers := make([]browser.PoolWorker, 0, poolSize)
	for i := 0; i < poolSize; i++ {
		w, err := cdpworker.NewWorkerWithConfig(cfg.Browser.ChromiumPath, cfg.Browser.Headless, cfg)
		if err != nil {
			log.Fatalf("create browser worker %d: %v", i, err)
		}
		poolWorkers = append(poolWorkers, w)
	}
	// Factory for replacing crashed workers.
	workerFactory := func() (browser.PoolWorker, error) {
		return cdpworker.NewWorkerWithConfig(cfg.Browser.ChromiumPath, cfg.Browser.Headless, cfg)
	}
	workerPool, err := browser.NewPoolWithFactory(poolWorkers, workerFactory)
	if err != nil {
		log.Fatalf("create worker pool: %v", err)
	}
	defer workerPool.Close()
	log.Printf("browser worker pool: %d worker(s)", poolSize)

	// Auth token store.
	tokenStore := auth.NewInMemoryStore()
	if cfg.Auth.Enabled && cfg.Auth.StaticToken != "" {
		tokenStore.Add(cfg.Auth.StaticToken, "admin")
		log.Printf("auth enabled; static token loaded")
	} else if cfg.Auth.Enabled {
		// Load from disk or generate a new one (persists across restarts).
		tokenFile := env("AUTH_TOKEN_FILE", ".mininext_token")
		token, fresh, err := auth.LoadOrGenerateToken(tokenFile)
		if err != nil {
			log.Fatalf("auth token: %v", err)
		}
		tokenStore.Add(token, "admin")
		if fresh {
			log.Printf("auth enabled; new token generated and saved to %s: %s", tokenFile, token)
		} else {
			log.Printf("auth enabled; token loaded from %s", tokenFile)
		}
	}

	// Archive store (optional — only opened if archive.enabled).
	var archiveStore *archive.Store
	if cfg.Archive.Enabled {
		var archErr error
		archiveStore, archErr = archive.Open(cfg.Archive.DBPath)
		if archErr != nil {
			log.Fatalf("open archive store: %v", archErr)
		}
		defer archiveStore.Close()
		log.Printf("archive store opened: %s", cfg.Archive.DBPath)
	}

	// Session manager with background expiry.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr := session.NewManager(ctx, workerPool, cfg)

	// Start worker health monitor (checks every 30s, replaces crashed workers).
	workerPool.StartHealthMonitor(ctx, 30*time.Second)

	// Start filter list refresh loop for all workers in the pool.
	if cfg.AdBlock.Enabled && len(cfg.AdBlock.FilterListURLs) > 0 {
		for _, pw := range poolWorkers {
			if w, ok := pw.(*cdpworker.Worker); ok {
				if m := w.AdMatcher(); m != nil {
					adblock.StartRefreshLoop(ctx, adblock.FilterListConfig{
						URLs:         cfg.AdBlock.FilterListURLs,
						RefreshHours: cfg.AdBlock.FilterListRefreshH,
						CacheDir:     cfg.AdBlock.FilterListCacheDir,
					}, m)
					break // all workers share domain list from config; one refresh loop is enough
				}
			}
		}
	}

	// HTTP router.
	webHandler := http.FileServer(http.Dir("web/"))
	router := api.NewRouter(mgr, cfg, tokenStore, archiveStore, workerPool, webHandler)

	srv := &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server.
	go func() {
		if cfg.Server.HTTPSEnabled && cfg.Server.CertFile != "" {
			log.Printf("MiniNext listening on %s (HTTPS)", cfg.Server.ListenAddr)
			if err := srv.ListenAndServeTLS(cfg.Server.CertFile, cfg.Server.KeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("server: %v", err)
			}
		} else {
			log.Printf("MiniNext listening on %s (HTTP)", cfg.Server.ListenAddr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("server: %v", err)
			}
		}
	}()

	// SIGHUP reloads config (idle timeout, tab limits, etc.) without restart.
	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGHUP)
	go func() {
		for range reload {
			newCfg := config.Load()
			mgr.UpdateConfig(newCfg)
			log.Printf("config reloaded (idle_timeout=%s, max_tabs=%d)",
				newCfg.Session.IdleTimeout, newCfg.Session.MaxTabs)
		}
	}()

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	cancel() // stop expiry loop
	signal.Stop(reload)
	close(reload)

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}
}
