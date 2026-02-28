package conversation

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

func (s *Store) LoadAsyncTaskState() ([]byte, error) {
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
		raw := meta.Get([]byte(metaAsyncTaskState))
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

func (s *Store) SaveAsyncTaskState(payload []byte) error {
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
			return meta.Delete([]byte(metaAsyncTaskState))
		}
		return meta.Put([]byte(metaAsyncTaskState), payload)
	})
}
