package memory

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

func (s *Store) ListAudits(limit int) []AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]AuditEntry, 0, 64)
	_ = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketAudit))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var entry AuditEntry
			if err := json.Unmarshal(v, &entry); err != nil {
				return nil
			}
			entries = append(entries, entry)
			return nil
		})
	})
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		}
		return entries[i].ID > entries[j].ID
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}

func (s *Store) RollbackNode(path string) (Node, error) {
	path, err := normalizePath(path)
	if err != nil {
		return Node{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		target   *Node
		latestAt time.Time
	)
	if err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketAudit))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var entry AuditEntry
			if err := json.Unmarshal(v, &entry); err != nil {
				return nil
			}
			if entry.Before == nil {
				return nil
			}
			if strings.TrimSpace(entry.Path) != path && strings.TrimSpace(entry.FromPath) != path {
				return nil
			}
			if target == nil || entry.CreatedAt.After(latestAt) {
				clone := *entry.Before
				target = &clone
				latestAt = entry.CreatedAt
			}
			return nil
		})
	}); err != nil {
		return Node{}, err
	}
	if target == nil || strings.TrimSpace(target.Path) == "" {
		return Node{}, fmt.Errorf("no rollback snapshot found for %s", path)
	}

	refs := []Ref{{Kind: "rollback", Value: time.Now().UTC().Format(time.RFC3339)}}
	if target.Content != nil {
		refs = append(refs, target.Content.Refs...)
	}
	node, err := s.upsertNodeLocked(UpsertRequest{
		Mode:          "replace",
		Path:          path,
		Title:         target.Title,
		Type:          target.Type,
		SchemaKind:    target.SchemaKind,
		SchemaVersion: target.SchemaVersion,
		Tags:          target.Tags,
		Source:        "manual",
		Confidence:    target.Confidence,
		Summary:       valueOrEmpty(target.Content, func(c *FileContent) string { return c.Summary }),
		Facts:         valueOrEmpty(target.Content, func(c *FileContent) []string { return c.Facts }),
		Sections:      valueOrEmpty(target.Content, func(c *FileContent) []Section { return c.Sections }),
		Refs:          refs,
	})
	if err != nil {
		return Node{}, err
	}
	_ = s.appendAuditLocked(AuditEntry{
		Action:    "rollback",
		Path:      path,
		Detail:    "rollback to previous audit snapshot",
		After:     cloneNodePtr(&node),
		CreatedAt: time.Now().UTC(),
	})
	return node, nil
}

func valueOrEmpty[T any](content *FileContent, getter func(*FileContent) T) T {
	var zero T
	if content == nil {
		return zero
	}
	return getter(content)
}

func cloneNodePtr(node *Node) *Node {
	if node == nil {
		return nil
	}
	clone := *node
	if node.Content != nil {
		contentClone := *node.Content
		contentClone.Facts = append([]string(nil), node.Content.Facts...)
		contentClone.Sections = append([]Section(nil), node.Content.Sections...)
		contentClone.Refs = append([]Ref(nil), node.Content.Refs...)
		clone.Content = &contentClone
	}
	clone.Tags = append([]string(nil), node.Tags...)
	return &clone
}

func refValue(refs []Ref, kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	for _, ref := range refs {
		if strings.ToLower(strings.TrimSpace(ref.Kind)) == kind {
			return strings.TrimSpace(ref.Value)
		}
	}
	return ""
}

func (s *Store) appendAuditLocked(entry AuditEntry) error {
	if s.db == nil {
		return nil
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = buildAuditID(entry.CreatedAt)
	}
	entry.Action = strings.TrimSpace(entry.Action)
	if entry.Action == "" {
		entry.Action = "unknown"
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketAudit))
		if b == nil {
			return fmt.Errorf("memory audit bucket missing")
		}
		return b.Put([]byte(entry.ID), payload)
	})
}

func buildAuditID(now time.Time) string {
	return fmt.Sprintf("audit-%s-%d", now.UTC().Format("20060102-150405"), now.UTC().UnixNano())
}
