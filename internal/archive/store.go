// Package archive provides persistent storage for saved page snapshots.
// Snapshots are stored as compressed MBPF in a bbolt database, keyed by UUID.
package archive

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketMeta = []byte("archive_meta")
	bucketData = []byte("archive_data")
	bucketUser = []byte("user_index") // key: "{userID}\x00{archiveID}" → archiveID
)

// Meta holds the stored metadata for one archived page.
type Meta struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	FaviconURL string    `json:"favicon_url,omitempty"`
	Format     string    `json:"format"`
	Size       int       `json:"size"`
	CreatedAt  time.Time `json:"created_at"`
}

// Store is a bbolt-backed archive store. All methods are safe for concurrent use.
type Store struct {
	db *bolt.DB
}

// Open opens (or creates) the bbolt archive database at path.
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open archive db: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, bkt := range [][]byte{bucketMeta, bucketData, bucketUser} {
			if _, err := tx.CreateBucketIfNotExists(bkt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("init archive buckets: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// Save stores a new archive entry. If the user already has maxPerUser archives,
// the oldest one is deleted first (0 = unlimited).
func (s *Store) Save(meta Meta, data []byte, maxPerUser int) error {
	if maxPerUser > 0 {
		if err := s.enforceLimit(meta.UserID, maxPerUser-1); err != nil {
			return err
		}
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		key := []byte(meta.ID)
		if err := tx.Bucket(bucketMeta).Put(key, metaBytes); err != nil {
			return err
		}
		if err := tx.Bucket(bucketData).Put(key, data); err != nil {
			return err
		}
		userKey := userIndexKey(meta.UserID, meta.ID)
		return tx.Bucket(bucketUser).Put(userKey, key)
	})
}

// List returns all archive metadata for a user, newest first (by CreatedAt).
func (s *Store) List(userID string) ([]Meta, error) {
	metas, err := s.metasForUser(userID)
	if err != nil {
		return nil, err
	}
	// Sort newest first.
	sortMetasByTime(metas, false)
	return metas, nil
}

// Get returns metadata + raw payload for a single archive entry.
// Returns ErrNotFound if the ID doesn't exist or belongs to a different user.
func (s *Store) Get(id, userID string) (*Meta, []byte, error) {
	var meta *Meta
	var data []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(bucketMeta).Get([]byte(id))
		if raw == nil {
			return ErrNotFound
		}
		var m Meta
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		if m.UserID != userID {
			return ErrForbidden
		}
		meta = &m
		// Copy data out of bbolt page before the transaction closes.
		src := tx.Bucket(bucketData).Get([]byte(id))
		if src != nil {
			data = make([]byte, len(src))
			copy(data, src)
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return meta, data, nil
}

// Delete removes an archive entry. Returns ErrNotFound / ErrForbidden as appropriate.
func (s *Store) Delete(id, userID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		raw := tx.Bucket(bucketMeta).Get([]byte(id))
		if raw == nil {
			return ErrNotFound
		}
		var m Meta
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		if m.UserID != userID {
			return ErrForbidden
		}
		if err := tx.Bucket(bucketMeta).Delete([]byte(id)); err != nil {
			return err
		}
		if err := tx.Bucket(bucketData).Delete([]byte(id)); err != nil {
			return err
		}
		return tx.Bucket(bucketUser).Delete(userIndexKey(userID, id))
	})
}

// CountForUser returns how many archives a user has.
func (s *Store) CountForUser(userID string) int {
	count := 0
	prefix := []byte(userID + "\x00")
	_ = s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketUser).Cursor()
		for k, _ := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, _ = c.Next() {
			count++
		}
		return nil
	})
	return count
}

// metasForUser fetches all metadata entries for a user in arbitrary order.
func (s *Store) metasForUser(userID string) ([]Meta, error) {
	var ids []string
	prefix := []byte(userID + "\x00")
	if err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketUser).Cursor()
		for k, v := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, v = c.Next() {
			ids = append(ids, string(v))
		}
		return nil
	}); err != nil {
		return nil, err
	}

	metas := make([]Meta, 0, len(ids))
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bucketMeta)
		for _, id := range ids {
			raw := bkt.Get([]byte(id))
			if raw == nil {
				continue
			}
			var m Meta
			if err := json.Unmarshal(raw, &m); err == nil {
				metas = append(metas, m)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return metas, nil
}

// enforceLimit deletes the oldest archives by CreatedAt until count <= max.
func (s *Store) enforceLimit(userID string, max int) error {
	metas, err := s.metasForUser(userID)
	if err != nil {
		return err
	}
	if len(metas) <= max {
		return nil
	}
	// Sort oldest first, delete the excess.
	sortMetasByTime(metas, true)
	for i := 0; len(metas)-i > max; i++ {
		oldest := metas[i]
		if err := s.db.Update(func(tx *bolt.Tx) error {
			tx.Bucket(bucketMeta).Delete([]byte(oldest.ID))
			tx.Bucket(bucketData).Delete([]byte(oldest.ID))
			return tx.Bucket(bucketUser).Delete(userIndexKey(userID, oldest.ID))
		}); err != nil {
			return err
		}
	}
	return nil
}

// sortMetasByTime sorts metas in-place. ascending=true → oldest first.
// Uses a simple insertion sort (N is small, ≤ max_per_user).
func sortMetasByTime(metas []Meta, ascending bool) {
	for i := 1; i < len(metas); i++ {
		for j := i; j > 0; j-- {
			a, b := metas[j-1].CreatedAt, metas[j].CreatedAt
			swap := ascending && a.After(b) || !ascending && b.After(a)
			if !swap {
				break
			}
			metas[j-1], metas[j] = metas[j], metas[j-1]
		}
	}
}

func userIndexKey(userID, archiveID string) []byte {
	return []byte(userID + "\x00" + archiveID)
}

var (
	ErrNotFound  = errors.New("archive not found")
	ErrForbidden = errors.New("forbidden")
)
