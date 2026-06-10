package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all server configuration.
type Config struct {
	Server   ServerConfig
	Auth     AuthConfig
	Browser  BrowserConfig
	Session  SessionConfig
	Encoding EncodingConfig
	Images   ImageConfig
}

type ServerConfig struct {
	ListenAddr   string
	HTTPSEnabled bool
	CertFile     string
	KeyFile      string
}

type AuthConfig struct {
	Enabled    bool
	StaticToken string // single static API token for Phase 1
}

type BrowserConfig struct {
	Engine        string // "chromium"
	ChromiumPath  string // empty = auto-detect
	WorkerPoolMin int
	WorkerPoolMax int
	Headless      bool
}

type SessionConfig struct {
	IdleTimeout time.Duration
	MaxTabs     int
}

type EncodingConfig struct {
	DefaultPageFormat   string // "minidom-text" or "mbpf"
	AllowMinidomText    bool
	DefaultCompression  string // "none", "gzip", "brotli"
}

type ImageConfig struct {
	DefaultFormat  string // "jpeg", "webp", "png", "gif"
	DefaultQuality string // "high", "medium", "low"
	MaxWidth       int
	MaxHeight      int
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			ListenAddr:   env("LISTEN", ":8080"),
			HTTPSEnabled: envBool("HTTPS_ENABLED", false),
			CertFile:     env("TLS_CERT", ""),
			KeyFile:      env("TLS_KEY", ""),
		},
		Auth: AuthConfig{
			Enabled:     envBool("AUTH_ENABLED", false),
			StaticToken: env("AUTH_TOKEN", ""),
		},
		Browser: BrowserConfig{
			Engine:        env("BROWSER_ENGINE", "chromium"),
			ChromiumPath:  env("CHROMIUM_PATH", ""),
			WorkerPoolMin: envInt("WORKER_POOL_MIN", 1),
			WorkerPoolMax: envInt("WORKER_POOL_MAX", 4),
			Headless:      envBool("BROWSER_HEADLESS", true),
		},
		Session: SessionConfig{
			IdleTimeout: envDuration("IDLE_TIMEOUT", 10*time.Minute),
			MaxTabs:     envInt("MAX_TABS", 10),
		},
		Encoding: EncodingConfig{
			DefaultPageFormat:  env("DEFAULT_PAGE_FORMAT", "minidom-text"),
			AllowMinidomText:   envBool("ALLOW_MINIDOM_TEXT", true),
			DefaultCompression: env("DEFAULT_COMPRESSION", "gzip"),
		},
		Images: ImageConfig{
			DefaultFormat:  env("IMAGE_FORMAT", "jpeg"),
			DefaultQuality: env("IMAGE_QUALITY", "medium"),
			MaxWidth:       envInt("IMAGE_MAX_WIDTH", 800),
			MaxHeight:      envInt("IMAGE_MAX_HEIGHT", 1200),
		},
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
