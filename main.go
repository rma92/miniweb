package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/miniweb/internal/api"
	"github.com/user/miniweb/internal/auth"
	cdpworker "github.com/user/miniweb/internal/browser/chromedp"
	"github.com/user/miniweb/internal/config"
	"github.com/user/miniweb/internal/session"
)

func main() {
	cfg := config.Load()

	// Browser worker.
	worker, err := cdpworker.NewWorker(cfg.Browser.ChromiumPath, cfg.Browser.Headless)
	if err != nil {
		log.Fatalf("create browser worker: %v", err)
	}
	defer worker.Close()

	// Auth token store.
	tokenStore := auth.NewInMemoryStore()
	if cfg.Auth.Enabled && cfg.Auth.StaticToken != "" {
		tokenStore.Add(cfg.Auth.StaticToken, "admin")
		log.Printf("auth enabled; static token loaded")
	} else if cfg.Auth.Enabled {
		// Auto-generate a token and print it so the operator knows it.
		token, err := auth.GenerateToken()
		if err != nil {
			log.Fatalf("generate auth token: %v", err)
		}
		tokenStore.Add(token, "admin")
		log.Printf("auth enabled; generated token: %s", token)
	}

	// Session manager with background expiry.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr := session.NewManager(ctx, worker, cfg)

	// HTTP router.
	webHandler := http.FileServer(http.Dir("web/"))
	router := api.NewRouter(mgr, cfg, tokenStore, webHandler)

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
