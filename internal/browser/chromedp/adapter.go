package chromedp

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/user/miniweb/internal/adblock"
	"github.com/user/miniweb/internal/browser"
	"github.com/user/miniweb/internal/config"
	"github.com/user/miniweb/internal/minidom"
)

// Worker implements browser.BrowserWorker using chromedp.
type Worker struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	mu          sync.RWMutex
	sessions    map[browser.SessionHandle]*cdpSession
	adMatcher   *adblock.Matcher // nil when ad blocking is disabled
}

type cdpSession struct {
	handle   browser.SessionHandle
	profile  browser.DeviceProfile
	sleeping bool
	// adBlockEnabled is shared across all tabs in the session. Using an atomic
	// so the CDP event listener goroutines can read it lock-free.
	adBlockEnabled atomic.Bool

	mu       sync.RWMutex
	tabs     map[browser.TabHandle]*cdpTab
	lastSnap map[browser.TabHandle]*minidom.PageSnapshot
	savedURL map[browser.TabHandle]string
}

type cdpTab struct {
	handle  browser.TabHandle
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewWorker creates a Worker with a shared Chromium allocator process.
func NewWorker(chromiumPath string, headless bool) (*Worker, error) {
	return NewWorkerWithConfig(chromiumPath, headless, nil)
}

// NewWorkerWithConfig creates a Worker, optionally enabling ad blocking from cfg.
func NewWorkerWithConfig(chromiumPath string, headless bool, cfg *config.Config) (*Worker, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-background-networking", true),
	)
	if !headless {
		opts = append(opts, chromedp.Flag("headless", false))
	}
	if chromiumPath != "" {
		opts = append(opts, chromedp.ExecPath(chromiumPath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	w := &Worker{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		sessions:    make(map[browser.SessionHandle]*cdpSession),
	}
	if cfg != nil && cfg.AdBlock.Enabled {
		w.adMatcher = adblock.NewMatcher(cfg.AdBlock.ExtraDomains)
		log.Printf("ad blocking enabled (%d built-in domains)", len(adblock.BuiltinDomains))
	}
	return w, nil
}

// Close shuts down the shared allocator.
func (w *Worker) Close() {
	w.allocCancel()
}

// Healthy performs a lightweight CDP round-trip to verify the Chromium process
// is still alive and responsive. Returns false on any error or timeout.
func (w *Worker) Healthy() bool {
	ctx, cancel := context.WithTimeout(w.allocCtx, 5*time.Second)
	defer cancel()
	tabCtx, tabCancel := chromedp.NewContext(ctx)
	defer tabCancel()
	var result int
	err := chromedp.Run(tabCtx, chromedp.Evaluate(`1+1`, &result))
	return err == nil
}

// AdMatcher returns the Matcher used for ad blocking, or nil if disabled.
func (w *Worker) AdMatcher() *adblock.Matcher {
	return w.adMatcher
}

// CreateSession creates a new isolated browser context.
func (w *Worker) CreateSession(profile browser.DeviceProfile) (browser.SessionHandle, error) {
	handle := browser.SessionHandle(newID("sess"))

	sess := &cdpSession{
		handle:   handle,
		profile:  profile,
		tabs:     make(map[browser.TabHandle]*cdpTab),
		lastSnap: make(map[browser.TabHandle]*minidom.PageSnapshot),
		savedURL: make(map[browser.TabHandle]string),
	}
	// Default adblock state: on when the server has a matcher configured.
	sess.adBlockEnabled.Store(w.adMatcher != nil)

	w.mu.Lock()
	w.sessions[handle] = sess
	w.mu.Unlock()

	return handle, nil
}

// OpenTab opens a new tab in the session and navigates to url.
func (w *Worker) OpenTab(session browser.SessionHandle, url string) (browser.TabHandle, error) {
	sess, err := w.getSession(session)
	if err != nil {
		return "", err
	}

	tabHandle := browser.TabHandle(newID("tab"))

	// Each tab gets its own browser context (= isolated profile/cookies).
	// Within a session, tabs share cookies by sharing the same allocCtx.
	// chromedp.NewContext creates a new browser target (tab), not a new profile.
	tabCtx, tabCancel := chromedp.NewContext(w.allocCtx, chromedp.WithLogf(log.Printf))

	tab := &cdpTab{
		handle: tabHandle,
		ctx:    tabCtx,
		cancel: tabCancel,
	}

	sess.mu.Lock()
	sess.tabs[tabHandle] = tab
	sess.savedURL[tabHandle] = url
	sess.mu.Unlock()

	// Enable ad blocking on the new tab. The session's adBlockEnabled atomic is
	// shared: flipping it (via SetAdBlock) takes effect immediately for all open
	// tabs in the session.
	if w.adMatcher != nil {
		if err := enableAdBlocking(tabCtx, w.adMatcher, &sess.adBlockEnabled); err != nil {
			log.Printf("adblock setup for tab %s: %v", tabHandle, err)
		}
	}

	if url != "" {
		if err := chromedp.Run(tabCtx,
			chromedp.Navigate(url),
			chromedp.WaitReady("body", chromedp.ByQuery),
		); err != nil {
			log.Printf("OpenTab navigate %s: %v", url, err)
			// Non-fatal: tab is open but page may have failed to load.
		}
	}

	return tabHandle, nil
}

// Navigate navigates the tab to a new URL.
func (w *Worker) Navigate(tab browser.TabHandle, url string) error {
	t, sess, err := w.getTab(tab)
	if err != nil {
		return err
	}

	if err := chromedp.Run(t.ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		return wrapNavError(url, err)
	}

	sess.mu.Lock()
	sess.savedURL[tab] = url
	sess.mu.Unlock()
	return nil
}

// Interact dispatches an interaction event on the tab.
func (w *Worker) Interact(tab browser.TabHandle, event browser.InteractionEvent) error {
	t, sess, err := w.getTab(tab)
	if err != nil {
		return err
	}

	sess.mu.RLock()
	snap := sess.lastSnap[tab]
	sess.mu.RUnlock()

	if snap == nil {
		return fmt.Errorf("no snapshot available for tab %s; call Snapshot first", tab)
	}

	switch event.Type {
	case "click", "tap":
		return ClickElement(t.ctx, event.ElementID, snap)
	case "input", "change":
		return SetInputValue(t.ctx, event.ElementID, event.Value, snap)
	case "submit":
		return SubmitForm(t.ctx, event.ElementID, nil, snap)
	default:
		return fmt.Errorf("unsupported event type: %s", event.Type)
	}
}

// Snapshot extracts the current rendered state of the tab.
func (w *Worker) Snapshot(tab browser.TabHandle, opts browser.SnapshotOptions) (*minidom.PageSnapshot, error) {
	t, sess, err := w.getTab(tab)
	if err != nil {
		return nil, err
	}

	var snap *minidom.PageSnapshot
	if opts.RenderingProfile == "flow" {
		snap, err = ExtractCurrentFlow(t.ctx)
	} else {
		snap, err = extractCurrent(t.ctx)
	}
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	sess.lastSnap[tab] = snap
	if snap.URL != "" {
		sess.savedURL[tab] = snap.URL
	}
	sess.mu.Unlock()

	return snap, nil
}

// CloseTab closes a single tab.
func (w *Worker) CloseTab(tab browser.TabHandle) error {
	_, sess, err := w.getTab(tab)
	if err != nil {
		return err
	}

	sess.mu.Lock()
	if t, ok := sess.tabs[tab]; ok {
		t.cancel()
		delete(sess.tabs, tab)
		delete(sess.lastSnap, tab)
		delete(sess.savedURL, tab)
	}
	sess.mu.Unlock()
	return nil
}

// SleepSession cancels all tab contexts to free renderer memory while keeping
// session metadata (URLs, last snapshots) in memory.
func (w *Worker) SleepSession(session browser.SessionHandle) error {
	sess, err := w.getSession(session)
	if err != nil {
		return err
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.sleeping {
		return nil
	}

	for _, tab := range sess.tabs {
		tab.cancel()
	}
	sess.sleeping = true
	return nil
}

// ResumeSession recreates tab contexts and navigates back to saved URLs.
func (w *Worker) ResumeSession(session browser.SessionHandle) error {
	sess, err := w.getSession(session)
	if err != nil {
		return err
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if !sess.sleeping {
		return nil
	}

	// Recreate all tabs.
	for handle, oldTab := range sess.tabs {
		url := sess.savedURL[handle]
		tabCtx, tabCancel := chromedp.NewContext(w.allocCtx, chromedp.WithLogf(log.Printf))
		newTab := &cdpTab{
			handle: oldTab.handle,
			ctx:    tabCtx,
			cancel: tabCancel,
		}
		sess.tabs[handle] = newTab

		if url != "" {
			go func(ctx context.Context, u string) {
				if err := chromedp.Run(ctx,
					chromedp.Navigate(u),
					chromedp.WaitReady("body", chromedp.ByQuery),
				); err != nil {
					log.Printf("resume navigate %s: %v", u, err)
				}
			}(tabCtx, url)
		}
	}
	sess.sleeping = false
	return nil
}

// DestroySession cancels all contexts and removes the session.
func (w *Worker) DestroySession(session browser.SessionHandle) error {
	w.mu.Lock()
	sess, ok := w.sessions[session]
	if !ok {
		w.mu.Unlock()
		return nil
	}
	delete(w.sessions, session)
	w.mu.Unlock()

	sess.mu.Lock()
	for _, tab := range sess.tabs {
		tab.cancel()
	}
	sess.mu.Unlock()
	return nil
}

// SetAdBlock enables or disables ad blocking for all tabs in the session.
// The change is immediately visible to any open tab's CDP event listener.
func (w *Worker) SetAdBlock(session browser.SessionHandle, enabled bool) error {
	if w.adMatcher == nil {
		return nil // server-side ad blocking not configured; no-op
	}
	sess, err := w.getSession(session)
	if err != nil {
		return err
	}
	sess.adBlockEnabled.Store(enabled)
	return nil
}

func (w *Worker) getSession(handle browser.SessionHandle) (*cdpSession, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	sess, ok := w.sessions[handle]
	if !ok {
		return nil, fmt.Errorf("session %s not found", handle)
	}
	return sess, nil
}

func (w *Worker) getTab(handle browser.TabHandle) (*cdpTab, *cdpSession, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, sess := range w.sessions {
		sess.mu.RLock()
		tab, ok := sess.tabs[handle]
		sess.mu.RUnlock()
		if ok {
			return tab, sess, nil
		}
	}
	return nil, nil, fmt.Errorf("tab %s not found", handle)
}
