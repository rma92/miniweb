package session

import (
	"sync"

	"github.com/user/miniweb/internal/browser"
	"github.com/user/miniweb/internal/minidom"
)

// TabEvent signals a navigation state change to SSE subscribers.
type TabEvent struct {
	Type    string `json:"type"`              // "ready" | "error"
	URL     string `json:"url,omitempty"`
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"` // populated for "error"
}

// Tab represents one browser tab within a session.
type Tab struct {
	ID         string
	SessionID  string
	Handle     browser.TabHandle
	CurrentURL string
	LastSnap   *minidom.PageSnapshot
	SnapID     int // monotonically increasing per tab

	evMu   sync.Mutex
	evSubs map[int]chan TabEvent
	evSeq  int
}

// Subscribe registers a listener for tab events. The returned cancel function
// must be called when the subscriber is done (it closes the channel).
func (t *Tab) Subscribe() (<-chan TabEvent, func()) {
	t.evMu.Lock()
	if t.evSubs == nil {
		t.evSubs = make(map[int]chan TabEvent)
	}
	id := t.evSeq
	t.evSeq++
	ch := make(chan TabEvent, 8)
	t.evSubs[id] = ch
	t.evMu.Unlock()

	return ch, func() {
		t.evMu.Lock()
		if _, ok := t.evSubs[id]; ok {
			delete(t.evSubs, id)
			close(ch)
		}
		t.evMu.Unlock()
	}
}

// publish fans an event out to all current subscribers. Non-blocking:
// a slow subscriber drops the event rather than stalling the caller.
func (t *Tab) publish(e TabEvent) {
	t.evMu.Lock()
	for _, ch := range t.evSubs {
		select {
		case ch <- e:
		default:
		}
	}
	t.evMu.Unlock()
}
