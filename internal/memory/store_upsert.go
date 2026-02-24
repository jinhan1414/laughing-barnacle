package memory

import (
	"fmt"
	"strings"
	"time"
)

func (s *Store) UpsertNode(req UpsertRequest) (Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertNodeLocked(req)
}

func (s *Store) upsertNodeLocked(req UpsertRequest) (Node, error) {
	path, err := normalizePath(req.Path)
	if err != nil {
		return Node{}, err
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "patch"
	}
	if mode != "patch" && mode != "replace" {
		return Node{}, fmt.Errorf("upsert mode must be patch or replace")
	}
	nodeType := req.Type
	if nodeType == "" {
		nodeType = NodeTypeFile
		if path == "/" {
			nodeType = NodeTypeDir
		}
	}
	if nodeType != NodeTypeDir && nodeType != NodeTypeFile {
		return Node{}, fmt.Errorf("invalid node type")
	}
	if path == "/" {
		nodeType = NodeTypeDir
	}

	existing, found, err := s.readNodeLocked(path)
	if err != nil {
		return Node{}, err
	}
	if found && req.ExpectedRevision > 0 && existing.Revision != req.ExpectedRevision {
		return Node{}, ErrRevisionConflict
	}

	now := time.Now().UTC()
	node := existing
	if !found {
		node = Node{ID: buildNodeID(path, now), Path: path, Type: nodeType, CreatedAt: now, Revision: 1}
	} else {
		node.Revision++
	}
	if mode == "replace" {
		node.Content = nil
		node.Tags = nil
	}
	if v := strings.TrimSpace(req.Title); v != "" {
		node.Title = trimRunes(v, 120)
	} else if strings.TrimSpace(node.Title) == "" {
		node.Title = pathTitle(path)
	}
	if v := strings.TrimSpace(req.SchemaKind); v != "" {
		node.SchemaKind = trimRunes(v, 60)
	} else if strings.TrimSpace(node.SchemaKind) == "" {
		node.SchemaKind = "generic"
	}
	if req.SchemaVersion > 0 {
		node.SchemaVersion = req.SchemaVersion
	} else if node.SchemaVersion <= 0 {
		node.SchemaVersion = 1
	}
	if mode == "replace" || req.Tags != nil {
		node.Tags = normalizeStringList(req.Tags, 24, 40)
	}
	if v := strings.TrimSpace(req.Source); v != "" {
		node.Source = trimRunes(v, 24)
	} else if strings.TrimSpace(node.Source) == "" {
		node.Source = "system"
	}
	if req.Confidence > 0 {
		node.Confidence = clamp(req.Confidence, 0, 1)
	} else if node.Confidence <= 0 {
		node.Confidence = 1
	}
	node.Type = nodeType
	if nodeType == NodeTypeFile {
		content := node.Content
		if content == nil || mode == "replace" {
			content = &FileContent{}
		}
		if mode == "replace" || strings.TrimSpace(req.Summary) != "" {
			content.Summary = trimRunes(req.Summary, 360)
		}
		if mode == "replace" || req.Facts != nil {
			content.Facts = normalizeStringList(req.Facts, 64, 200)
		}
		if mode == "replace" || req.Sections != nil {
			content.Sections = normalizeSections(req.Sections)
		}
		if mode == "replace" || req.Refs != nil {
			content.Refs = normalizeRefs(req.Refs)
		}
		if strings.TrimSpace(content.Summary) == "" {
			content.Summary = "(无摘要)"
		}
		node.Content = content
	} else {
		node.Content = nil
	}
	node.UpdatedAt = now

	if err := s.ensureParentLocked(path); err != nil {
		return Node{}, err
	}
	if err := s.writeNodeLocked(node); err != nil {
		return Node{}, err
	}
	if !found && path != "/" {
		if err := s.addChildLocked(parentPath(path), path); err != nil {
			return Node{}, err
		}
	}
	after := cloneNodePtr(&node)
	var before *Node
	if found {
		before = cloneNodePtr(&existing)
	}
	_ = s.appendAuditLocked(AuditEntry{
		Action:    "upsert",
		Path:      path,
		Detail:    "mode=" + mode,
		Before:    before,
		After:     after,
		CreatedAt: now,
	})
	return node, nil
}

func (s *Store) ensureParentLocked(path string) error {
	if path == "/" {
		return nil
	}
	pp := parentPath(path)
	if pp == "" {
		return nil
	}
	parent, found, err := s.readNodeLocked(pp)
	if err != nil {
		return err
	}
	if found {
		if parent.Type != NodeTypeDir {
			return ErrPathConflict
		}
		return nil
	}
	_, err = s.upsertNodeLocked(UpsertRequest{Mode: "patch", Path: pp, Type: NodeTypeDir, Title: pathTitle(pp), SchemaKind: "namespace", SchemaVersion: 1, Source: "system", Confidence: 1})
	return err
}
