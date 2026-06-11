package browser

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user/miniweb/internal/minidom"
)

// Pool is a BrowserWorker that distributes sessions across N underlying workers.
// New sessions are assigned to the worker with the fewest active sessions.
// All subsequent calls on a session or tab are routed to the same worker.
type Pool struct {
	mu         sync.RWMutex
	workers    []PoolWorker
	sessMap    map[SessionHandle]int         // session → worker index
	tabMap     map[TabHandle]int             // tab → worker index
	sessTabs   map[SessionHandle][]TabHandle // session → owned tabs (for cleanup)
	counts     []atomic.Int64                // active session count per worker
	factory    WorkerFactory                 // nil = no replacement on crash
	unhealthy  []atomic.Bool                 // true if worker[i] has failed health check
}

// PoolWorker extends BrowserWorker with lifecycle methods needed by the pool.
type PoolWorker interface {
	BrowserWorker
	Close()
}

// HealthCheckable is an optional interface a PoolWorker can implement.
// The pool calls Healthy() during health checks to detect crashed processes.
type HealthCheckable interface {
	Healthy() bool
}

// WorkerFactory creates a fresh PoolWorker of the same type (for replacement after crash).
type WorkerFactory func() (PoolWorker, error)

// NewPool wraps a pre-created slice of workers in a pool.
func NewPool(workers []PoolWorker) (*Pool, error) {
	return NewPoolWithFactory(workers, nil)
}

// NewPoolWithFactory creates a pool that can replace crashed workers using factory.
func NewPoolWithFactory(workers []PoolWorker, factory WorkerFactory) (*Pool, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("pool requires at least one worker")
	}
	p := &Pool{
		workers:   workers,
		sessMap:   make(map[SessionHandle]int),
		tabMap:    make(map[TabHandle]int),
		sessTabs:  make(map[SessionHandle][]TabHandle),
		counts:    make([]atomic.Int64, len(workers)),
		unhealthy: make([]atomic.Bool, len(workers)),
		factory:   factory,
	}
	return p, nil
}

// StartHealthMonitor starts a background goroutine that checks all workers every
// interval. Unhealthy workers (if factory is set) are replaced with fresh ones;
// sessions on failed workers are evicted so clients receive errors and reconnect.
func (p *Pool) StartHealthMonitor(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.checkAllWorkers()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (p *Pool) checkAllWorkers() {
	for i, w := range p.workers {
		hc, ok := w.(HealthCheckable)
		if !ok {
			continue
		}
		if hc.Healthy() {
			p.unhealthy[i].Store(false)
			continue
		}
		// Worker is unhealthy.
		if p.unhealthy[i].Swap(true) {
			continue // already marked; replacement may already be in progress
		}
		log.Printf("pool: worker %d failed health check", i)

		if p.factory == nil {
			log.Printf("pool: worker %d unhealthy (no factory configured for replacement)", i)
			continue
		}

		// Create replacement worker.
		newW, err := p.factory()
		if err != nil {
			log.Printf("pool: failed to create replacement for worker %d: %v", i, err)
			continue
		}

		// Evict all sessions on the failed worker — clients will get errors
		// and need to create new sessions.
		p.mu.Lock()
		old := p.workers[i]
		p.workers[i] = newW
		var evicted []SessionHandle
		for sess, idx := range p.sessMap {
			if idx == i {
				evicted = append(evicted, sess)
			}
		}
		for _, sess := range evicted {
			for _, tab := range p.sessTabs[sess] {
				delete(p.tabMap, tab)
			}
			delete(p.sessTabs, sess)
			delete(p.sessMap, sess)
		}
		p.counts[i].Store(0)
		p.mu.Unlock()

		p.unhealthy[i].Store(false)
		old.Close()
		log.Printf("pool: worker %d replaced (%d sessions evicted)", i, len(evicted))
	}
}

// WorkerStats returns per-worker session counts and health state for admin use.
func (p *Pool) WorkerStats() []WorkerStat {
	stats := make([]WorkerStat, len(p.workers))
	for i := range p.workers {
		stats[i] = WorkerStat{
			Index:     i,
			Sessions:  int(p.counts[i].Load()),
			Unhealthy: p.unhealthy[i].Load(),
		}
	}
	return stats
}

// WorkerStat holds the health and load state of one pool worker.
type WorkerStat struct {
	Index     int  `json:"index"`
	Sessions  int  `json:"sessions"`
	Unhealthy bool `json:"unhealthy"`
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

// leastLoaded returns the index of the healthiest, least-loaded worker.
// Prefers healthy workers; falls back to unhealthy ones if all are unhealthy.
func (p *Pool) leastLoaded() int {
	best := -1
	bestCount := int64(1<<62)
	// First pass: healthy workers only.
	for i := range p.counts {
		if p.unhealthy[i].Load() {
			continue
		}
		if c := p.counts[i].Load(); best < 0 || c < bestCount {
			bestCount = c
			best = i
		}
	}
	if best >= 0 {
		return best
	}
	// All workers are unhealthy — pick least-loaded anyway.
	best = 0
	bestCount = p.counts[0].Load()
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
