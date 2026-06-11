package session

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/user/miniweb/internal/browser"
	"github.com/user/miniweb/internal/config"
	"github.com/user/miniweb/internal/metrics"
	"github.com/user/miniweb/internal/minidom"
	"github.com/user/miniweb/internal/session/persist"
)

// Manager owns all active sessions and drives the idle-expiry loop.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	worker   browser.BrowserWorker
	cfg      atomic.Pointer[config.Config]
	snapSeq  atomic.Int64 // global snapshot ID counter
	store    *persist.Store
}

// NewManager creates a Manager and starts the background expiry goroutine.
func NewManager(ctx context.Context, worker browser.BrowserWorker, cfg *config.Config) *Manager {
	m := &Manager{
		sessions: make(map[string]*Session),
		worker:   worker,
	}
	m.cfg.Store(cfg)
	go m.expiryLoop(ctx)
	return m
}

// SetPersistStore attaches a persistence store. Must be called before
// RestoreSessions and before the first session is created.
func (m *Manager) SetPersistStore(s *persist.Store) {
	m.store = s
}

// RestoreSessions re-creates sessions from persisted records on startup.
// It should be called once after SetPersistStore and before serving requests.
func (m *Manager) RestoreSessions(records []persist.SessionRecord) {
	for _, rec := range records {
		profile, ok := browser.DefaultProfiles[rec.ProfileID]
		if !ok {
			profile = browser.DefaultProfiles["phone-modern"]
		}

		handle, err := m.worker.CreateSession(profile)
		if err != nil {
			log.Printf("restore session %s: create browser session: %v", rec.SessionID, err)
			continue
		}

		sess := &Session{
			ID:             rec.SessionID,
			UserID:         rec.UserID,
			Handle:         handle,
			Profile:        profile,
			State:          StateActive,
			AdBlockEnabled: rec.AdBlockEnabled,
			CreatedAt:      rec.CreatedAt,
			LastActive:     time.Now(),
			tabs:           make(map[string]*Tab),
		}

		m.mu.Lock()
		m.sessions[sess.ID] = sess
		m.mu.Unlock()
		metrics.ActiveSessions.Inc()

		restored := 0
		for _, tr := range rec.Tabs {
			tabHandle, err := m.worker.OpenTab(handle, tr.CurrentURL)
			if err != nil {
				log.Printf("restore tab %s (session %s): %v", tr.TabID, rec.SessionID, err)
				continue
			}
			tab := &Tab{
				ID:         tr.TabID,
				SessionID:  sess.ID,
				Handle:     tabHandle,
				CurrentURL: tr.CurrentURL,
			}
			sess.addTab(tab)
			restored++
		}
		log.Printf("restored session %s (%d/%d tab(s))", rec.SessionID, restored, len(rec.Tabs))
	}
}

// persistSession saves the current state of a session to the store (no-op if no store).
func (m *Manager) persistSession(sess *Session) {
	if m.store == nil {
		return
	}
	sess.mu.RLock()
	tabs := make([]persist.TabRecord, 0, len(sess.tabs))
	for _, t := range sess.tabs {
		if t.CurrentURL != "" {
			tabs = append(tabs, persist.TabRecord{
				TabID:      t.ID,
				CurrentURL: t.CurrentURL,
			})
		}
	}
	rec := persist.SessionRecord{
		SessionID:      sess.ID,
		UserID:         sess.UserID,
		ProfileID:      sess.Profile.Name,
		AdBlockEnabled: sess.AdBlockEnabled,
		CreatedAt:      sess.CreatedAt,
		LastActive:     sess.LastActive,
		Tabs:           tabs,
	}
	sess.mu.RUnlock()

	if err := m.store.Save(rec); err != nil {
		log.Printf("persist session %s: %v", sess.ID, err)
	}
}

// UpdateConfig atomically replaces the manager's config (used for SIGHUP reload).
func (m *Manager) UpdateConfig(cfg *config.Config) {
	m.cfg.Store(cfg)
}

// CreateSession creates a new session for the given user and device profile.
func (m *Manager) CreateSession(userID string, profile browser.DeviceProfile) (*Session, error) {
	handle, err := m.worker.CreateSession(profile)
	if err != nil {
		return nil, fmt.Errorf("create browser session: %w", err)
	}

	id := "sess_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	sess := newSession(id, userID, handle, profile)

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	metrics.ActiveSessions.Inc()
	m.persistSession(sess)
	return sess, nil
}

// GetSession returns a session by ID. Pass empty userID to skip ownership check
// (used internally by the expiry reaper).
func (m *Manager) GetSession(id, userID string) (*Session, error) {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	if sess.State == StateDead {
		return nil, ErrNotFound
	}
	if userID != "" && sess.UserID != userID {
		return nil, ErrForbidden
	}
	return sess, nil
}

// OpenTab opens a new tab in the session and navigates to url.
func (m *Manager) OpenTab(sess *Session, url string) (*Tab, error) {
	cfg := m.cfg.Load()
	if cfg.Session.MaxTabs > 0 && len(sess.TabIDs()) >= cfg.Session.MaxTabs {
		return nil, fmt.Errorf("maximum tabs per session (%d) reached", cfg.Session.MaxTabs)
	}

	handle, err := m.worker.OpenTab(sess.Handle, url)
	if err != nil {
		return nil, fmt.Errorf("open tab: %w", err)
	}

	tabID := "tab_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tab := &Tab{
		ID:         tabID,
		SessionID:  sess.ID,
		Handle:     handle,
		CurrentURL: url,
	}
	sess.addTab(tab)
	sess.touch()
	m.persistSession(sess)
	return tab, nil
}

// GetTab returns a tab from a session. Exposed for use in the api package.
func (m *Manager) GetTab(sess *Session, tabID string) (*Tab, bool) {
	return sess.getTab(tabID)
}

// Navigate navigates a tab to a new URL.
func (m *Manager) Navigate(sess *Session, tabID, url string) error {
	tab, ok := sess.getTab(tabID)
	if !ok {
		return ErrNotFound
	}
	if err := m.worker.Navigate(tab.Handle, url); err != nil {
		return err
	}
	tab.CurrentURL = url
	sess.touch()
	m.persistSession(sess)
	return nil
}

// NavigateAsync starts navigation in a background goroutine and returns
// immediately. Completion is signalled by a TabEvent pushed to the tab's
// subscribers. The caller should have an SSE stream open to receive it.
func (m *Manager) NavigateAsync(sess *Session, tabID, url string) error {
	tab, ok := sess.getTab(tabID)
	if !ok {
		return ErrNotFound
	}
	go m.doNavigateAsync(sess, tab, url)
	return nil
}

func (m *Manager) doNavigateAsync(sess *Session, tab *Tab, url string) {
	if err := m.worker.Navigate(tab.Handle, url); err != nil {
		tab.publish(TabEvent{Type: "error", Message: err.Error()})
		return
	}
	tab.CurrentURL = url
	sess.touch()
	m.persistSession(sess)
	tab.publish(TabEvent{Type: "ready", URL: url})
}

// Snapshot extracts the current page snapshot for a tab.
func (m *Manager) Snapshot(sess *Session, tabID string, opts browser.SnapshotOptions) (*minidom.PageSnapshot, error) {
	tab, ok := sess.getTab(tabID)
	if !ok {
		return nil, ErrNotFound
	}

	snap, err := m.worker.Snapshot(tab.Handle, opts)
	if err != nil {
		return nil, err
	}

	tab.SnapID = int(m.snapSeq.Add(1))
	snap.SnapshotID = tab.SnapID
	tab.LastSnap = snap
	urlChanged := snap.URL != "" && snap.URL != tab.CurrentURL
	if snap.URL != "" {
		tab.CurrentURL = snap.URL
	}
	sess.touch()
	if urlChanged {
		m.persistSession(sess)
	}
	return snap, nil
}

// Interact dispatches an interaction event on a tab.
func (m *Manager) Interact(sess *Session, tabID string, event browser.InteractionEvent) error {
	tab, ok := sess.getTab(tabID)
	if !ok {
		return ErrNotFound
	}
	if err := m.worker.Interact(tab.Handle, event); err != nil {
		return err
	}
	sess.touch()
	return nil
}

// ListAllSessions returns all non-dead sessions for admin use.
// Unlike GetSession, this bypasses user ownership checks.
func (m *Manager) ListAllSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		s.mu.RLock()
		state := s.State
		s.mu.RUnlock()
		if state != StateDead {
			out = append(out, s)
		}
	}
	return out
}

// ForceDeleteSession destroys a session regardless of ownership (admin only).
func (m *Manager) ForceDeleteSession(id string) error {
	return m.DeleteSession(id, "") // empty userID skips ownership check
}

// GetLastSnap returns the last snapshot for a tab if its ID matches sinceID.
// Used by the delta snapshot path to retrieve the base for diffing.
func (m *Manager) GetLastSnap(sess *Session, tabID string, sinceID int) *minidom.PageSnapshot {
	tab, ok := sess.getTab(tabID)
	if !ok || tab.LastSnap == nil {
		return nil
	}
	if tab.LastSnap.SnapshotID != sinceID {
		return nil
	}
	return tab.LastSnap
}

// GetResource returns image bytes for a resource in the tab's last snapshot.
func (m *Manager) GetResource(sess *Session, tabID, resourceID string) (*minidom.ResourceRef, error) {
	tab, ok := sess.getTab(tabID)
	if !ok {
		return nil, ErrNotFound
	}
	if tab.LastSnap == nil {
		return nil, fmt.Errorf("no snapshot available")
	}
	for i := range tab.LastSnap.Resources {
		if tab.LastSnap.Resources[i].ResourceID == resourceID {
			return &tab.LastSnap.Resources[i], nil
		}
	}
	return nil, ErrNotFound
}

// CloseTab closes a tab.
func (m *Manager) CloseTab(sess *Session, tabID string) error {
	tab, ok := sess.getTab(tabID)
	if !ok {
		return ErrNotFound
	}
	if err := m.worker.CloseTab(tab.Handle); err != nil {
		return err
	}
	sess.removeTab(tabID)
	sess.touch()
	m.persistSession(sess)
	return nil
}

// SleepSession sleeps a session.
func (m *Manager) SleepSession(id, userID string) error {
	sess, err := m.GetSession(id, userID)
	if err != nil {
		return err
	}
	if err := m.worker.SleepSession(sess.Handle); err != nil {
		return err
	}
	sess.mu.Lock()
	sess.State = StateSleeping
	sess.mu.Unlock()
	return nil
}

// ResumeSession wakes a sleeping session.
func (m *Manager) ResumeSession(id, userID string) error {
	sess, err := m.GetSession(id, userID)
	if err != nil {
		return err
	}
	if err := m.worker.ResumeSession(sess.Handle); err != nil {
		return err
	}
	sess.mu.Lock()
	sess.State = StateActive
	sess.mu.Unlock()
	sess.touch()
	return nil
}

// SetAdBlock enables or disables ad blocking for a session.
func (m *Manager) SetAdBlock(sess *Session, enabled bool) error {
	if err := m.worker.SetAdBlock(sess.Handle, enabled); err != nil {
		return err
	}
	sess.mu.Lock()
	sess.AdBlockEnabled = enabled
	sess.mu.Unlock()
	sess.touch()
	m.persistSession(sess)
	return nil
}

// DeleteSession destroys a session and its browser resources.
func (m *Manager) DeleteSession(id, userID string) error {
	sess, err := m.GetSession(id, userID)
	if err != nil {
		return err
	}

	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()

	sess.mu.Lock()
	sess.State = StateDead
	sess.mu.Unlock()

	if m.store != nil {
		if err := m.store.Delete(id); err != nil {
			log.Printf("persist delete session %s: %v", id, err)
		}
	}

	metrics.ActiveSessions.Dec()
	return m.worker.DestroySession(sess.Handle)
}

// expiryLoop periodically reaps sessions that have exceeded the idle timeout.
func (m *Manager) expiryLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.reapExpired()
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) reapExpired() {
	timeout := m.cfg.Load().Session.IdleTimeout
	if timeout <= 0 {
		return
	}

	m.mu.RLock()
	var expired []string
	for id, sess := range m.sessions {
		sess.mu.RLock()
		idle := time.Since(sess.LastActive)
		state := sess.State
		sess.mu.RUnlock()
		if state != StateDead && idle > timeout {
			expired = append(expired, id)
		}
	}
	m.mu.RUnlock()

	for _, id := range expired {
		if err := m.DeleteSession(id, ""); err != nil {
			// DeleteSession with empty userID skips ownership check.
		}
	}
}

var (
	ErrNotFound  = fmt.Errorf("not found")
	ErrForbidden = fmt.Errorf("forbidden")
)
