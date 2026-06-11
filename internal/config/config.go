package config

import (
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all server configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Browser  BrowserConfig  `yaml:"browser"`
	Session  SessionConfig  `yaml:"session"`
	Encoding EncodingConfig `yaml:"encoding"`
	Images   ImageConfig    `yaml:"images"`
	AdBlock  AdBlockConfig  `yaml:"adblock"`
	Archive  ArchiveConfig  `yaml:"archive"`
}

type ArchiveConfig struct {
	Enabled      bool   `yaml:"enabled"`
	DBPath       string `yaml:"db_path"`
	MaxPerUser   int    `yaml:"max_per_user"`
	AdminToken   string `yaml:"admin_token"` // also used as the admin API token
}

type AdBlockConfig struct {
	Enabled             bool     `yaml:"enabled"`
	ExtraDomains        []string `yaml:"extra_domains"`
	FilterListURLs      []string `yaml:"filter_list_urls"`
	FilterListRefreshH  int      `yaml:"filter_list_refresh_hours"`
	FilterListCacheDir  string   `yaml:"filter_list_cache_dir"`
}

type ServerConfig struct {
	ListenAddr        string  `yaml:"listen"`
	HTTPSEnabled      bool    `yaml:"https_enabled"`
	CertFile          string  `yaml:"cert_file"`
	KeyFile           string  `yaml:"key_file"`
	RateLimitPerSec   float64 `yaml:"rate_limit_per_sec"`   // 0 = disabled
	RateLimitBurst    float64 `yaml:"rate_limit_burst"`
}

type AuthConfig struct {
	Enabled     bool   `yaml:"enabled"`
	StaticToken string `yaml:"static_token"`
}

type BrowserConfig struct {
	Engine        string `yaml:"engine"`
	ChromiumPath  string `yaml:"chromium_path"`
	WorkerPoolMin int    `yaml:"worker_pool_min"`
	WorkerPoolMax int    `yaml:"worker_pool_max"`
	Headless      bool   `yaml:"headless"`
}

type SessionConfig struct {
	IdleTimeout time.Duration `yaml:"-"` // parsed from IdleTimeoutStr
	IdleTimeoutStr string     `yaml:"idle_timeout"`
	MaxTabs     int           `yaml:"max_tabs"`
}

type EncodingConfig struct {
	DefaultPageFormat  string `yaml:"default_page_format"`
	AllowMinidomText   bool   `yaml:"allow_minidom_text"`
	DefaultCompression string `yaml:"default_compression"`
}

type ImageConfig struct {
	DefaultFormat  string `yaml:"default_format"`
	DefaultQuality string `yaml:"default_quality"`
	MaxWidth       int    `yaml:"max_width"`
	MaxHeight      int    `yaml:"max_height"`
}

// Load reads config from a YAML file (if present), then overlays env-var overrides.
// The YAML file path defaults to "config.yaml" and can be changed via CONFIG_FILE.
func Load() *Config {
	cfg := defaults()

	// Try loading from YAML file.
	cfgFile := env("CONFIG_FILE", "config.yaml")
	if data, err := os.ReadFile(cfgFile); err == nil {
		// Ignore parse errors — just fall through to defaults + env vars.
		_ = yaml.Unmarshal(data, cfg)
		// Parse string duration from YAML.
		if cfg.Session.IdleTimeoutStr != "" {
			if d, err := time.ParseDuration(cfg.Session.IdleTimeoutStr); err == nil {
				cfg.Session.IdleTimeout = d
			}
		}
	}

	// Env-var overrides (always take precedence over file).
	applyEnvOverrides(cfg)
	return cfg
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			ListenAddr:      ":8080",
			HTTPSEnabled:    false,
			RateLimitPerSec: 0, // disabled by default
			RateLimitBurst:  30,
		},
		Auth: AuthConfig{
			Enabled: false,
		},
		Browser: BrowserConfig{
			Engine:        "chromium",
			WorkerPoolMin: 1,
			WorkerPoolMax: 4,
			Headless:      true,
		},
		Session: SessionConfig{
			IdleTimeout: 10 * time.Minute,
			MaxTabs:     10,
		},
		Encoding: EncodingConfig{
			DefaultPageFormat:  "minidom-text",
			AllowMinidomText:   true,
			DefaultCompression: "gzip",
		},
		Images: ImageConfig{
			DefaultFormat:  "jpeg",
			DefaultQuality: "medium",
			MaxWidth:       800,
			MaxHeight:      1200,
		},
		AdBlock: AdBlockConfig{
			Enabled: false,
		},
		Archive: ArchiveConfig{
			Enabled:    false,
			DBPath:     "archives.db",
			MaxPerUser: 100,
		},
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("LISTEN"); v != "" {
		cfg.Server.ListenAddr = v
	}
	if v := os.Getenv("HTTPS_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Server.HTTPSEnabled = b
		}
	}
	if v := os.Getenv("TLS_CERT"); v != "" {
		cfg.Server.CertFile = v
	}
	if v := os.Getenv("TLS_KEY"); v != "" {
		cfg.Server.KeyFile = v
	}
	if v := os.Getenv("AUTH_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Auth.Enabled = b
		}
	}
	if v := os.Getenv("AUTH_TOKEN"); v != "" {
		cfg.Auth.StaticToken = v
	}
	if v := os.Getenv("BROWSER_ENGINE"); v != "" {
		cfg.Browser.Engine = v
	}
	if v := os.Getenv("CHROMIUM_PATH"); v != "" {
		cfg.Browser.ChromiumPath = v
	}
	if v := os.Getenv("WORKER_POOL_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Browser.WorkerPoolMin = n
		}
	}
	if v := os.Getenv("WORKER_POOL_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Browser.WorkerPoolMax = n
		}
	}
	if v := os.Getenv("BROWSER_HEADLESS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Browser.Headless = b
		}
	}
	if v := os.Getenv("IDLE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Session.IdleTimeout = d
		}
	}
	if v := os.Getenv("MAX_TABS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Session.MaxTabs = n
		}
	}
	if v := os.Getenv("DEFAULT_PAGE_FORMAT"); v != "" {
		cfg.Encoding.DefaultPageFormat = v
	}
	if v := os.Getenv("ALLOW_MINIDOM_TEXT"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Encoding.AllowMinidomText = b
		}
	}
	if v := os.Getenv("DEFAULT_COMPRESSION"); v != "" {
		cfg.Encoding.DefaultCompression = v
	}
	if v := os.Getenv("IMAGE_FORMAT"); v != "" {
		cfg.Images.DefaultFormat = v
	}
	if v := os.Getenv("IMAGE_QUALITY"); v != "" {
		cfg.Images.DefaultQuality = v
	}
	if v := os.Getenv("IMAGE_MAX_WIDTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Images.MaxWidth = n
		}
	}
	if v := os.Getenv("IMAGE_MAX_HEIGHT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Images.MaxHeight = n
		}
	}
	if v := os.Getenv("ADBLOCK_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AdBlock.Enabled = b
		}
	}
	if v := os.Getenv("ARCHIVE_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Archive.Enabled = b
		}
	}
	if v := os.Getenv("ARCHIVE_DB"); v != "" {
		cfg.Archive.DBPath = v
	}
	if v := os.Getenv("ADMIN_TOKEN"); v != "" {
		cfg.Archive.AdminToken = v
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
