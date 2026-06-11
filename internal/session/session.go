package session

import (
	"sync"
	"time"

	"github.com/user/miniweb/internal/browser"
)

// State is the lifecycle state of a session.
type State int

const (
	StateActive   State = iota
	StateSleeping
	StateDead
)

// Session represents one remote browsing environment for a user.
type Session struct {
	ID              string
	UserID          string
	Handle          browser.SessionHandle
	Profile         browser.DeviceProfile
	State           State
	AdBlockEnabled  bool // per-session ad blocking toggle
	CreatedAt       time.Time
	LastActive      time.Time

	mu   sync.RWMutex
	tabs map[string]*Tab // tab ID → Tab
}

func newSession(id, userID string, handle browser.SessionHandle, profile browser.DeviceProfile) *Session {
	now := time.Now()
	return &Session{
		ID:             id,
		UserID:         userID,
		Handle:         handle,
		Profile:        profile,
		State:          StateActive,
		AdBlockEnabled: true, // on by default; worker's SetAdBlock call controls actual CDP state
		CreatedAt:      now,
		LastActive:     now,
		tabs:           make(map[string]*Tab),
	}
}

func (s *Session) addTab(tab *Tab) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tabs[tab.ID] = tab
}

func (s *Session) getTab(tabID string) (*Tab, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tabs[tabID]
	return t, ok
}

func (s *Session) removeTab(tabID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tabs, tabID)
}

// TabIDs returns a snapshot of the tab IDs.
func (s *Session) TabIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.tabs))
	for id := range s.tabs {
		ids = append(ids, id)
	}
	return ids
}

func (s *Session) touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActive = time.Now()
}

// Info returns a snapshot of session metadata for admin/status use.
func (s *Session) Info() SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stateStr := "active"
	if s.State == StateSleeping {
		stateStr = "sleeping"
	}

	tabs := make([]TabInfo, 0, len(s.tabs))
	for _, t := range s.tabs {
		tabs = append(tabs, TabInfo{TabID: t.ID, CurrentURL: t.CurrentURL})
	}

	return SessionInfo{
		SessionID:      s.ID,
		UserID:         s.UserID,
		State:          stateStr,
		CreatedAt:      s.CreatedAt,
		LastActive:     s.LastActive,
		Tabs:           tabs,
		AdBlockEnabled: s.AdBlockEnabled,
	}
}

// SessionInfo is a serialisable snapshot of session metadata.
type SessionInfo struct {
	SessionID      string    `json:"session_id"`
	UserID         string    `json:"user_id"`
	State          string    `json:"state"`
	CreatedAt      time.Time `json:"created_at"`
	LastActive     time.Time `json:"last_active"`
	Tabs           []TabInfo `json:"tabs"`
	AdBlockEnabled bool      `json:"adblock_enabled"`
}

// TabInfo is a serialisable snapshot of tab metadata.
type TabInfo struct {
	TabID      string `json:"tab_id"`
	CurrentURL string `json:"current_url"`
}
