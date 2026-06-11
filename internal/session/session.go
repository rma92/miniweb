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
