package conversation

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

func (s *Store) deleteAsyncTaskStateLocked() error {
	if s.db == nil {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(bucketMeta))
		if meta == nil {
			return fmt.Errorf("conversation db schema missing")
		}
		return meta.Delete([]byte(metaAsyncTaskState))
	})
}

func (s *Store) deleteChatTurnStateLocked() error {
	if s.db == nil {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(bucketMeta))
		if meta == nil {
			return fmt.Errorf("conversation db schema missing")
		}
		return meta.Delete([]byte(metaChatTurnState))
	})
}
