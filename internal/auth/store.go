package auth

import "sync"

// Store maps bearer tokens to user IDs.
type Store interface {
	Lookup(token string) (userID string, ok bool)
	Add(token, userID string)
	Remove(token string)
}

// InMemoryStore is a simple in-memory token store for Phase 1.
type InMemoryStore struct {
	mu     sync.RWMutex
	tokens map[string]string // token → userID
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{tokens: make(map[string]string)}
}

func (s *InMemoryStore) Lookup(token string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	uid, ok := s.tokens[token]
	return uid, ok
}

func (s *InMemoryStore) Add(token, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = userID
}

func (s *InMemoryStore) Remove(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
}
