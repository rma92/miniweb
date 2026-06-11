package session

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/user/miniweb/internal/browser"
	"github.com/user/miniweb/internal/config"
	"github.com/user/miniweb/internal/minidom"
)

// Manager owns all active sessions and drives the idle-expiry loop.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	worker   browser.BrowserWorker
	cfg      atomic.Pointer[config.Config]
	snapSeq  atomic.Int64 // global snapshot ID counter
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
	return tab, nil
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
	return nil
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
	if snap.URL != "" {
		tab.CurrentURL = snap.URL
	}
	sess.touch()
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
