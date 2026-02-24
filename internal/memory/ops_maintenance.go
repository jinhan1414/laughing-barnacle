package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

func (s *Store) RunMaintenance(now time.Time, trashTTL, failedRetryAfter time.Duration) (MaintenanceReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if trashTTL <= 0 {
		trashTTL = 30 * 24 * time.Hour
	}
	if failedRetryAfter <= 0 {
		failedRetryAfter = 2 * time.Minute
	}

	report := MaintenanceReport{RanAt: now}
	retried, err := s.retryFailedSegments(now, failedRetryAfter)
	if err != nil {
		return report, err
	}
	report.RetriedSegments = retried

	if _, err := s.CloseIdleSegments(now, 0, 0, 0); err != nil {
		return report, err
	}
	if err := s.ProcessClosedSegments(now); err != nil {
		return report, err
	}

	removed, err := s.cleanupTrash(now, trashTTL)
	if err != nil {
		return report, err
	}
	report.RemovedTrashNodes = removed

	repaired, err := s.rebuildChildrenIndex()
	if err != nil {
		return report, err
	}
	report.RepairedChildren = repaired

	s.mu.Lock()
	_ = s.appendAuditLocked(AuditEntry{
		Action:    "maintenance",
		Detail:    fmt.Sprintf("retried=%d removed_trash=%d repaired_children=%d", report.RetriedSegments, report.RemovedTrashNodes, report.RepairedChildren),
		CreatedAt: now,
	})
	s.mu.Unlock()
	return report, nil
}

func (s *Store) retryFailedSegments(now time.Time, retryAfter time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	segments, err := s.listSegmentsLocked()
	if err != nil {
		return 0, err
	}
	retried := 0
	for _, seg := range segments {
		if seg.Status != SegmentStatusFailed {
			continue
		}
		if !seg.UpdatedAt.IsZero() && now.Sub(seg.UpdatedAt) < retryAfter {
			continue
		}
		seg.Status = SegmentStatusClosed
		seg.Error = ""
		seg.RetryCount++
		seg.UpdatedAt = now
		if err := s.writeSegmentLocked(seg); err != nil {
			return retried, err
		}
		retried++
	}
	return retried, nil
}

func (s *Store) cleanupTrash(now time.Time, ttl time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanupTrashLocked(now, ttl)
}

func (s *Store) cleanupTrashLocked(now time.Time, ttl time.Duration) (int, error) {
	if ttl <= 0 {
		return 0, nil
	}
	cutoff := now.Add(-ttl)
	candidates := make([]string, 0, 16)
	if err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNodes))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var node Node
			if err := json.Unmarshal(v, &node); err != nil {
				return nil
			}
			if !strings.HasPrefix(node.Path, "/inbox/trash/") {
				return nil
			}
			if node.UpdatedAt.IsZero() || node.UpdatedAt.After(cutoff) {
				return nil
			}
			candidates = append(candidates, node.Path)
			return nil
		})
	}); err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	roots := dedupeSubtreeRoots(candidates)
	removed := 0
	for _, root := range roots {
		if err := s.deleteSubtreeLocked(root); err != nil {
			if errors.Is(err, ErrNodeNotFound) {
				continue
			}
			return removed, err
		}
		_ = s.removeChildLocked(parentPath(root), root)
		removed++
	}
	return removed, nil
}

func dedupeSubtreeRoots(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	roots := make([]string, 0, len(paths))
	for _, path := range paths {
		skip := false
		for _, root := range roots {
			if path == root || strings.HasPrefix(path, root+"/") {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		roots = append(roots, path)
	}
	return roots
}

func (s *Store) rebuildChildrenIndexLocked() (int, error) {
	nodes := map[string]Node{}
	if err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNodes))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var node Node
			if err := json.Unmarshal(v, &node); err != nil {
				return nil
			}
			if strings.TrimSpace(node.Path) != "" {
				nodes[node.Path] = node
			}
			return nil
		})
	}); err != nil {
		return 0, err
	}

	repaired := 0
	for path := range nodes {
		if path == "/" {
			continue
		}
		pp := parentPath(path)
		if pp == "" {
			continue
		}
		if _, ok := nodes[pp]; ok {
			continue
		}
		node, err := s.upsertNodeLocked(UpsertRequest{
			Mode:          "patch",
			Path:          pp,
			Type:          NodeTypeDir,
			Title:         pathTitle(pp),
			SchemaKind:    "namespace",
			SchemaVersion: 1,
			Source:        "system",
			Confidence:    1,
		})
		if err != nil {
			return repaired, err
		}
		nodes[pp] = node
		repaired++
	}

	expected := map[string][]string{}
	dirs := map[string]struct{}{"/": {}}
	for path, node := range nodes {
		if node.Type == NodeTypeDir {
			dirs[path] = struct{}{}
		}
		if path == "/" {
			continue
		}
		pp := parentPath(path)
		expected[pp] = append(expected[pp], path)
	}

	for dir := range dirs {
		if _, ok := expected[dir]; !ok {
			expected[dir] = nil
		}
	}

	current := map[string][]string{}
	if err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChildren))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var children []string
			if len(v) > 0 {
				_ = json.Unmarshal(v, &children)
			}
			current[string(k)] = children
			return nil
		})
	}); err != nil {
		return repaired, err
	}

	union := map[string]struct{}{}
	for dir := range expected {
		union[dir] = struct{}{}
	}
	for dir := range current {
		union[dir] = struct{}{}
	}

	for dir := range union {
		exp := normalizePathList(expected[dir])
		cur := normalizePathList(current[dir])
		if stringSliceEqual(exp, cur) {
			continue
		}
		if err := s.writeChildrenLocked(dir, exp); err != nil {
			return repaired, err
		}
		repaired++
	}
	return repaired, nil
}

func (s *Store) rebuildChildrenIndex() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rebuildChildrenIndexLocked()
}

func stringSliceEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
