package session

import (
	"github.com/user/miniweb/internal/browser"
	"github.com/user/miniweb/internal/minidom"
)

// Tab represents one browser tab within a session.
type Tab struct {
	ID         string
	SessionID  string
	Handle     browser.TabHandle
	CurrentURL string
	LastSnap   *minidom.PageSnapshot
	SnapID     int // monotonically increasing per tab
}
