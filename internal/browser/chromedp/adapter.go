package chromedp

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/chromedp/chromedp"
	"github.com/user/miniweb/internal/browser"
	"github.com/user/miniweb/internal/minidom"
)

// Worker implements browser.BrowserWorker using chromedp.
type Worker struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	mu          sync.RWMutex
	sessions    map[browser.SessionHandle]*cdpSession
}

type cdpSession struct {
	handle   browser.SessionHandle
	profile  browser.DeviceProfile
	sleeping bool

	// Each tab gets its own chromedp context derived from the session allocator.
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

	return &Worker{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		sessions:    make(map[browser.SessionHandle]*cdpSession),
	}, nil
}

// Close shuts down the shared allocator.
func (w *Worker) Close() {
	w.allocCancel()
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
		return fmt.Errorf("navigate %s: %w", url, err)
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
func (w *Worker) Snapshot(tab browser.TabHandle, _ browser.SnapshotOptions) (*minidom.PageSnapshot, error) {
	t, sess, err := w.getTab(tab)
	if err != nil {
		return nil, err
	}

	snap, err := extractCurrent(t.ctx)
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
