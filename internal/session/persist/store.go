// Package persist provides bbolt-backed persistence for session and tab state,
// allowing sessions to survive server restarts.
package persist

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

var bktSessions = []byte("sessions")

// TabRecord stores the minimum information needed to restore a tab.
type TabRecord struct {
	TabID      string `json:"tab_id"`
	CurrentURL string `json:"current_url"`
}

// SessionRecord stores the minimum information needed to restore a session.
type SessionRecord struct {
	SessionID      string      `json:"session_id"`
	UserID         string      `json:"user_id"`
	ProfileID      string      `json:"profile_id"` // device profile name key
	AdBlockEnabled bool        `json:"adblock_enabled"`
	CreatedAt      time.Time   `json:"created_at"`
	LastActive     time.Time   `json:"last_active"`
	Tabs           []TabRecord `json:"tabs"`
}

// Store persists session state to a bbolt database.
type Store struct {
	db *bolt.DB
}

// Open opens (or creates) the persistence database at path.
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	return s, db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bktSessions)
		return err
	})
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Save writes or overwrites a session record atomically.
func (s *Store) Save(rec SessionRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bktSessions).Put([]byte(rec.SessionID), data)
	})
}

// Delete removes a session record by ID.
func (s *Store) Delete(sessionID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bktSessions).Delete([]byte(sessionID))
	})
}

// LoadAll returns all stored session records. Corrupt records are silently skipped.
func (s *Store) LoadAll() ([]SessionRecord, error) {
	var out []SessionRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bktSessions).ForEach(func(_, v []byte) error {
			var rec SessionRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return nil
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}
