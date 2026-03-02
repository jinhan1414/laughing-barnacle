package conversation

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

func (s *Store) LoadAutonomousRunState() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, nil
	}
	var payload []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(bucketMeta))
		if meta == nil {
			return fmt.Errorf("conversation db schema missing")
		}
		raw := meta.Get([]byte(metaAutonomousRunState))
		if len(raw) == 0 {
			return nil
		}
		payload = append([]byte(nil), raw...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Store) SaveAutonomousRunState(payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(bucketMeta))
		if meta == nil {
			return fmt.Errorf("conversation db schema missing")
		}
		if len(payload) == 0 {
			return meta.Delete([]byte(metaAutonomousRunState))
		}
		return meta.Put([]byte(metaAutonomousRunState), payload)
	})
}

func (s *Store) deleteAutonomousRunStateLocked() error {
	if s.db == nil {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(bucketMeta))
		if meta == nil {
			return fmt.Errorf("conversation db schema missing")
		}
		return meta.Delete([]byte(metaAutonomousRunState))
	})
}
