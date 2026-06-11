package browser

import "github.com/user/miniweb/internal/minidom"

// SessionHandle is an opaque identifier for a remote browser session.
type SessionHandle string

// TabHandle is an opaque identifier for a tab within a session.
type TabHandle string

// DeviceProfile describes the viewport and UA to emulate.
type DeviceProfile struct {
	Name              string  `json:"profile_name"`
	ViewportWidth     int     `json:"viewport_width"`
	ViewportHeight    int     `json:"viewport_height"`
	DeviceScaleFactor float64 `json:"device_scale_factor"`
	UserAgent         string  `json:"user_agent"`
	Touch             bool    `json:"touch"`
	Mobile            bool    `json:"mobile"`
	ColorScheme       string  `json:"preferred_color_scheme"`
	AcceptLanguage    string  `json:"accept_language"`
}

// InteractionEvent describes a user interaction to replay on the remote browser.
type InteractionEvent struct {
	Type      string `json:"type"`       // click, input, change, submit, scroll, ...
	ElementID int    `json:"element_id"`
	Value     string `json:"value,omitempty"`
	ScrollX   int    `json:"scroll_x,omitempty"`
	ScrollY   int    `json:"scroll_y,omitempty"`
}

// SnapshotOptions controls the format and fidelity of a page snapshot.
type SnapshotOptions struct {
	Format           string // "minidom-text" or "mbpf"
	RenderingProfile string // "box" or "flow"
	ImageFormat      string // "jpeg", "webp", "png", "gif"
	ImageQuality     string // "high", "medium", "low"
	IncludeImages    bool
}

// BrowserWorker abstracts the remote browser engine.
type BrowserWorker interface {
	CreateSession(profile DeviceProfile) (SessionHandle, error)
	OpenTab(session SessionHandle, url string) (TabHandle, error)
	Navigate(tab TabHandle, url string) error
	Interact(tab TabHandle, event InteractionEvent) error
	Snapshot(tab TabHandle, options SnapshotOptions) (*minidom.PageSnapshot, error)
	CloseTab(tab TabHandle) error
	SleepSession(session SessionHandle) error
	ResumeSession(session SessionHandle) error
	DestroySession(session SessionHandle) error
	// SetAdBlock enables or disables ad blocking for a session.
	// The change takes effect immediately for any open tabs that have the CDP
	// Fetch listener active, and for all future tabs in the session.
	SetAdBlock(session SessionHandle, enabled bool) error
}

// DefaultProfiles provides the six built-in device profiles from the spec.
var DefaultProfiles = map[string]DeviceProfile{
	"phone-small": {
		Name: "phone-small", ViewportWidth: 360, ViewportHeight: 640,
		DeviceScaleFactor: 2, Touch: true, Mobile: true,
		ColorScheme: "light", AcceptLanguage: "en-US,en;q=0.9",
		UserAgent: "Mozilla/5.0 (Linux; Android 10; SM-A105F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
	},
	"phone-modern": {
		Name: "phone-modern", ViewportWidth: 390, ViewportHeight: 844,
		DeviceScaleFactor: 3, Touch: true, Mobile: true,
		ColorScheme: "light", AcceptLanguage: "en-US,en;q=0.9",
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
	},
	"tablet": {
		Name: "tablet", ViewportWidth: 768, ViewportHeight: 1024,
		DeviceScaleFactor: 2, Touch: true, Mobile: false,
		ColorScheme: "light", AcceptLanguage: "en-US,en;q=0.9",
		UserAgent: "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
	},
	"desktop-small": {
		Name: "desktop-small", ViewportWidth: 1280, ViewportHeight: 800,
		DeviceScaleFactor: 1, Touch: false, Mobile: false,
		ColorScheme: "light", AcceptLanguage: "en-US,en;q=0.9",
		UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	},
	"desktop-large": {
		Name: "desktop-large", ViewportWidth: 1920, ViewportHeight: 1080,
		DeviceScaleFactor: 1, Touch: false, Mobile: false,
		ColorScheme: "light", AcceptLanguage: "en-US,en;q=0.9",
		UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	},
	"custom": {
		Name: "custom", ViewportWidth: 1280, ViewportHeight: 800,
		DeviceScaleFactor: 1, Touch: false, Mobile: false,
		ColorScheme: "light", AcceptLanguage: "en-US,en;q=0.9",
		UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	},
}
