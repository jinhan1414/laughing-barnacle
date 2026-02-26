package memory

import (
	"errors"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

var defaultNamespacePaths = []string{
	"/",
	"/meta",
	"/profile",
	"/preferences",
	"/constraints",
	"/goals",
	"/projects",
	"/routines",
	"/conversation",
	"/conversation/archive",
	"/inbox",
	"/inbox/pending",
	"/inbox/reviewed",
	"/inbox/trash",
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.resetBucketsLocked(); err != nil {
		return fmt.Errorf("reset memory store buckets: %w", err)
	}
	if err := s.seedDefaultNamespacesLocked(); err != nil {
		return fmt.Errorf("seed memory default namespaces: %w", err)
	}
	return nil
}

func (s *Store) resetBucketsLocked() error {
	if s.db == nil {
		return fmt.Errorf("memory store is closed")
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range []string{
			bucketNodes,
			bucketChildren,
			bucketSegments,
			bucketMeta,
			bucketAudit,
		} {
			err := tx.DeleteBucket([]byte(bucket))
			if err != nil && !errors.Is(err, bolt.ErrBucketNotFound) {
				return err
			}
			if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) seedDefaultNamespacesLocked() error {
	for _, path := range defaultNamespacePaths {
		if _, err := s.upsertNodeLocked(UpsertRequest{
			Mode:          "patch",
			Path:          path,
			Type:          NodeTypeDir,
			Title:         pathTitle(path),
			SchemaKind:    "namespace",
			SchemaVersion: 1,
			Source:        "system",
			Confidence:    1,
		}); err != nil {
			return err
		}
	}
	return nil
}
