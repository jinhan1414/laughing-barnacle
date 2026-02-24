package memory

import (
	"fmt"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

func (s *Store) ListIndex(path string) ([]IndexItem, error) {
	path, err := normalizePath(path)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	node, found, err := s.readNodeLocked(path)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNodeNotFound
	}
	if node.Type != NodeTypeDir {
		return nil, fmt.Errorf("path %s is not directory", path)
	}
	children, err := s.readChildrenLocked(path)
	if err != nil {
		return nil, err
	}
	items := make([]IndexItem, 0, len(children))
	for _, childPath := range children {
		child, ok, rerr := s.readNodeLocked(childPath)
		if rerr != nil || !ok {
			continue
		}
		it := IndexItem{Path: child.Path, Title: child.Title, Type: child.Type, Revision: child.Revision, UpdatedAt: child.UpdatedAt}
		if child.Type == NodeTypeFile && child.Content != nil {
			it.Summary = child.Content.Summary
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return items[i].Path < items[j].Path
	})
	return items, nil
}

func (s *Store) ReadNode(path string) (Node, error) {
	path, err := normalizePath(path)
	if err != nil {
		return Node{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	node, found, err := s.readNodeLocked(path)
	if err != nil {
		return Node{}, err
	}
	if !found {
		return Node{}, ErrNodeNotFound
	}
	return node, nil
}

func (s *Store) ReadSection(path, sectionID string) (Section, error) {
	path, err := normalizePath(path)
	if err != nil {
		return Section{}, err
	}
	sectionID = strings.TrimSpace(sectionID)
	if sectionID == "" {
		return Section{}, ErrSectionNotFound
	}
	node, err := s.ReadNode(path)
	if err != nil {
		return Section{}, err
	}
	if node.Type != NodeTypeFile || node.Content == nil {
		return Section{}, ErrSectionNotFound
	}
	for _, sec := range node.Content.Sections {
		if strings.EqualFold(sec.ID, sectionID) {
			return sec, nil
		}
	}
	return Section{}, ErrSectionNotFound
}

func (s *Store) MoveNode(fromPath, toPath string, expectedRevision int64) error {
	fromPath, err := normalizePath(fromPath)
	if err != nil {
		return err
	}
	toPath, err = normalizePath(toPath)
	if err != nil {
		return err
	}
	if fromPath == "/" || toPath == "/" {
		return fmt.Errorf("root path cannot be moved")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fromNode, found, err := s.readNodeLocked(fromPath)
	if err != nil {
		return err
	}
	if !found {
		return ErrNodeNotFound
	}
	if expectedRevision > 0 && fromNode.Revision != expectedRevision {
		return ErrRevisionConflict
	}
	if _, exists, err := s.readNodeLocked(toPath); err != nil {
		return err
	} else if exists {
		return ErrPathConflict
	}
	return s.moveNodeLocked(fromPath, toPath)
}

func (s *Store) moveSubtreeLocked(fromPrefix, toPrefix string) error {
	mapping := map[string]string{}
	if err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNodes))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, _ []byte) error {
			oldPath := string(k)
			if oldPath == fromPrefix || strings.HasPrefix(oldPath, fromPrefix+"/") {
				mapping[oldPath] = toPrefix + strings.TrimPrefix(oldPath, fromPrefix)
			}
			return nil
		})
	}); err != nil {
		return err
	}
	if len(mapping) == 0 {
		return ErrNodeNotFound
	}
	ordered := make([]string, 0, len(mapping))
	for old := range mapping {
		ordered = append(ordered, old)
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) < len(ordered[j]) })
	for _, oldPath := range ordered {
		node, ok, err := s.readNodeLocked(oldPath)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		node.Path = mapping[oldPath]
		node.Revision++
		node.UpdatedAt = time.Now().UTC()
		if err := s.writeNodeLocked(node); err != nil {
			return err
		}
		if err := s.deleteNodeKeyLocked(oldPath); err != nil {
			return err
		}
		children, err := s.readChildrenLocked(oldPath)
		if err == nil && len(children) > 0 {
			newChildren := make([]string, 0, len(children))
			for _, c := range children {
				if mapped, ok := mapping[c]; ok {
					newChildren = append(newChildren, mapped)
				} else {
					newChildren = append(newChildren, c)
				}
			}
			if err := s.writeChildrenLocked(mapping[oldPath], newChildren); err != nil {
				return err
			}
			_ = s.deleteChildrenKeyLocked(oldPath)
		}
	}
	return nil
}

func (s *Store) DeleteNode(path string, soft bool) (string, error) {
	path, err := normalizePath(path)
	if err != nil {
		return "", err
	}
	if path == "/" {
		return "", fmt.Errorf("cannot delete root")
	}
	if soft {
		trash := fmt.Sprintf("/inbox/trash/%s-%s", time.Now().UTC().Format("20060102-150405"), encodePath(path))
		if err := s.MoveNode(path, trash, 0); err != nil {
			return "", err
		}
		return trash, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before, found, err := s.readNodeLocked(path)
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrNodeNotFound
	}
	if err := s.deleteSubtreeLocked(path); err != nil {
		return "", err
	}
	_ = s.removeChildLocked(parentPath(path), path)
	_ = s.appendAuditLocked(AuditEntry{
		Action:    "delete_hard",
		Path:      path,
		Before:    cloneNodePtr(&before),
		CreatedAt: time.Now().UTC(),
	})
	return "", nil
}

func (s *Store) deleteSubtreeLocked(prefix string) error {
	keys := make([]string, 0, 16)
	if err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNodes))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, _ []byte) error {
			path := string(k)
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				keys = append(keys, path)
			}
			return nil
		})
	}); err != nil {
		return err
	}
	if len(keys) == 0 {
		return ErrNodeNotFound
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, path := range keys {
		if err := s.deleteNodeKeyLocked(path); err != nil {
			return err
		}
		_ = s.deleteChildrenKeyLocked(path)
	}
	return nil
}
