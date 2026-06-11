package browser

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/user/miniweb/internal/minidom"
)

// Pool is a BrowserWorker that distributes sessions across N underlying workers.
// New sessions are assigned to the worker with the fewest active sessions.
// All subsequent calls on a session or tab are routed to the same worker.
type Pool struct {
	mu         sync.RWMutex
	workers    []PoolWorker
	sessMap    map[SessionHandle]int        // session → worker index
	tabMap     map[TabHandle]int            // tab → worker index
	sessTabs   map[SessionHandle][]TabHandle // session → owned tabs (for cleanup)
	counts     []atomic.Int64               // active session count per worker
}

// PoolWorker extends BrowserWorker with lifecycle methods needed by the pool.
type PoolWorker interface {
	BrowserWorker
	Close()
}

// NewPool wraps a pre-created slice of workers in a pool.
func NewPool(workers []PoolWorker) (*Pool, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("pool requires at least one worker")
	}
	p := &Pool{
		workers:  workers,
		sessMap:  make(map[SessionHandle]int),
		tabMap:   make(map[TabHandle]int),
		sessTabs: make(map[SessionHandle][]TabHandle),
		counts:   make([]atomic.Int64, len(workers)),
	}
	return p, nil
}

// Close shuts down all workers.
func (p *Pool) Close() {
	for _, w := range p.workers {
		w.Close()
	}
}

// WorkerCount returns the number of workers in the pool.
func (p *Pool) WorkerCount() int { return len(p.workers) }

// CreateSession assigns the session to the least-loaded worker and delegates.
func (p *Pool) CreateSession(profile DeviceProfile) (SessionHandle, error) {
	idx := p.leastLoaded()
	handle, err := p.workers[idx].CreateSession(profile)
	if err != nil {
		return "", err
	}
	p.mu.Lock()
	p.sessMap[handle] = idx
	p.sessTabs[handle] = nil
	p.mu.Unlock()
	p.counts[idx].Add(1)
	return handle, nil
}

func (p *Pool) OpenTab(session SessionHandle, url string) (TabHandle, error) {
	idx, err := p.workerIdxFor(session)
	if err != nil {
		return "", err
	}
	tab, err := p.workers[idx].OpenTab(session, url)
	if err != nil {
		return "", err
	}
	p.mu.Lock()
	p.tabMap[tab] = idx
	p.sessTabs[session] = append(p.sessTabs[session], tab)
	p.mu.Unlock()
	return tab, nil
}

func (p *Pool) Navigate(tab TabHandle, url string) error {
	w, err := p.workerForTab(tab)
	if err != nil {
		return err
	}
	return w.Navigate(tab, url)
}

func (p *Pool) Interact(tab TabHandle, event InteractionEvent) error {
	w, err := p.workerForTab(tab)
	if err != nil {
		return err
	}
	return w.Interact(tab, event)
}

func (p *Pool) Snapshot(tab TabHandle, opts SnapshotOptions) (*minidom.PageSnapshot, error) {
	w, err := p.workerForTab(tab)
	if err != nil {
		return nil, err
	}
	return w.Snapshot(tab, opts)
}

func (p *Pool) CloseTab(tab TabHandle) error {
	idx, err := p.workerIdxForTab(tab)
	if err != nil {
		return err
	}
	if err := p.workers[idx].CloseTab(tab); err != nil {
		return err
	}
	p.mu.Lock()
	delete(p.tabMap, tab)
	// Remove tab from sessTabs. We don't know the session here, so scan all.
	for sess, tabs := range p.sessTabs {
		for i, t := range tabs {
			if t == tab {
				p.sessTabs[sess] = append(tabs[:i], tabs[i+1:]...)
				break
			}
		}
	}
	p.mu.Unlock()
	return nil
}

func (p *Pool) SleepSession(session SessionHandle) error {
	w, err := p.workerFor(session)
	if err != nil {
		return err
	}
	return w.SleepSession(session)
}

func (p *Pool) ResumeSession(session SessionHandle) error {
	w, err := p.workerFor(session)
	if err != nil {
		return err
	}
	return w.ResumeSession(session)
}

func (p *Pool) DestroySession(session SessionHandle) error {
	idx, err := p.workerIdxFor(session)
	if err != nil {
		return err
	}
	if err := p.workers[idx].DestroySession(session); err != nil {
		return err
	}
	p.mu.Lock()
	for _, tab := range p.sessTabs[session] {
		delete(p.tabMap, tab)
	}
	delete(p.sessTabs, session)
	delete(p.sessMap, session)
	p.mu.Unlock()
	p.counts[idx].Add(-1)
	return nil
}

func (p *Pool) SetAdBlock(session SessionHandle, enabled bool) error {
	w, err := p.workerFor(session)
	if err != nil {
		return err
	}
	return w.SetAdBlock(session, enabled)
}

// leastLoaded returns the index of the worker with the fewest active sessions.
func (p *Pool) leastLoaded() int {
	best := 0
	bestCount := p.counts[0].Load()
	for i := 1; i < len(p.counts); i++ {
		if c := p.counts[i].Load(); c < bestCount {
			bestCount = c
			best = i
		}
	}
	return best
}

func (p *Pool) workerIdxFor(session SessionHandle) (int, error) {
	p.mu.RLock()
	idx, ok := p.sessMap[session]
	p.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("session %s not found in pool", session)
	}
	return idx, nil
}

func (p *Pool) workerFor(session SessionHandle) (BrowserWorker, error) {
	idx, err := p.workerIdxFor(session)
	if err != nil {
		return nil, err
	}
	return p.workers[idx], nil
}

func (p *Pool) workerIdxForTab(tab TabHandle) (int, error) {
	p.mu.RLock()
	idx, ok := p.tabMap[tab]
	p.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("tab %s not found in pool", tab)
	}
	return idx, nil
}

func (p *Pool) workerForTab(tab TabHandle) (BrowserWorker, error) {
	idx, err := p.workerIdxForTab(tab)
	if err != nil {
		return nil, err
	}
	return p.workers[idx], nil
}
