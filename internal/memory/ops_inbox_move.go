package memory

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (s *Store) ListInboxPending(limit int) []Node {
	nodes := s.ListNodes(0)
	out := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		if node.Type != NodeTypeFile {
			continue
		}
		if !strings.HasPrefix(node.Path, "/inbox/pending/") {
			continue
		}
		out = append(out, node)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Store) ReviewInboxCandidate(path, action string) (string, error) {
	path, err := normalizePath(path)
	if err != nil {
		return "", err
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		action = "confirm"
	}
	if action != "confirm" && action != "reject" {
		return "", fmt.Errorf("action must be confirm or reject")
	}
	if !strings.HasPrefix(path, "/inbox/pending/") {
		return "", fmt.Errorf("path must be in /inbox/pending")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	candidate, found, err := s.readNodeLocked(path)
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrNodeNotFound
	}
	if candidate.Type != NodeTypeFile || candidate.Content == nil {
		return "", fmt.Errorf("pending candidate must be file node")
	}

	if action == "reject" {
		trashPath := "/inbox/trash/" + time.Now().UTC().Format("20060102-150405") + "-" + encodePath(path)
		if err := s.moveNodeLocked(path, trashPath); err != nil {
			return "", err
		}
		_ = s.appendAuditLocked(AuditEntry{
			Action:    "inbox_review_reject",
			Path:      path,
			ToPath:    trashPath,
			Detail:    "candidate rejected",
			Before:    cloneNodePtr(&candidate),
			After:     nil,
			CreatedAt: time.Now().UTC(),
		})
		return trashPath, nil
	}

	targetPath := refValue(candidate.Content.Refs, "target_path")
	if strings.TrimSpace(targetPath) == "" {
		return "", fmt.Errorf("candidate target_path missing")
	}
	targetTitle := refValue(candidate.Content.Refs, "target_title")
	if strings.TrimSpace(targetTitle) == "" {
		targetTitle = candidate.Title
	}
	targetSchema := refValue(candidate.Content.Refs, "target_schema_kind")
	if strings.TrimSpace(targetSchema) == "" {
		targetSchema = "generic"
	}
	confidence := candidate.Confidence
	if raw := refValue(candidate.Content.Refs, "target_confidence"); strings.TrimSpace(raw) != "" {
		if parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(raw), 64); parseErr == nil {
			confidence = clamp(parsed, 0, 1)
		}
	}

	refs := append([]Ref(nil), candidate.Content.Refs...)
	refs = append(refs, Ref{Kind: "approved_from", Value: path})

	approvedNode, err := s.upsertNodeLocked(UpsertRequest{
		Mode:          "replace",
		Path:          targetPath,
		Title:         targetTitle,
		Type:          NodeTypeFile,
		SchemaKind:    targetSchema,
		SchemaVersion: 1,
		Source:        "manual",
		Confidence:    confidence,
		Summary:       candidate.Content.Summary,
		Facts:         candidate.Content.Facts,
		Sections:      candidate.Content.Sections,
		Refs:          refs,
	})
	if err != nil {
		return "", err
	}

	reviewedPath := "/inbox/reviewed/" + time.Now().UTC().Format("20060102-150405") + "-" + encodePath(path)
	if err := s.moveNodeLocked(path, reviewedPath); err != nil {
		return "", err
	}
	_ = s.appendAuditLocked(AuditEntry{
		Action:    "inbox_review_confirm",
		Path:      path,
		ToPath:    approvedNode.Path,
		Detail:    "candidate promoted to target namespace",
		Before:    cloneNodePtr(&candidate),
		After:     cloneNodePtr(&approvedNode),
		CreatedAt: time.Now().UTC(),
	})
	return approvedNode.Path, nil
}

func (s *Store) moveNodeLocked(fromPath, toPath string) error {
	if fromPath == "/" || toPath == "/" {
		return fmt.Errorf("root path cannot be moved")
	}
	fromNode, found, err := s.readNodeLocked(fromPath)
	if err != nil {
		return err
	}
	if !found {
		return ErrNodeNotFound
	}
	if _, exists, err := s.readNodeLocked(toPath); err != nil {
		return err
	} else if exists {
		return ErrPathConflict
	}
	if err := s.ensureParentLocked(toPath); err != nil {
		return err
	}
	if err := s.moveSubtreeLocked(fromPath, toPath); err != nil {
		return err
	}
	_ = s.removeChildLocked(parentPath(fromPath), fromPath)
	_ = s.addChildLocked(parentPath(toPath), toPath)
	if after, ok, _ := s.readNodeLocked(toPath); ok {
		_ = s.appendAuditLocked(AuditEntry{
			Action:    "move",
			Path:      toPath,
			FromPath:  fromPath,
			ToPath:    toPath,
			Detail:    "node moved",
			Before:    cloneNodePtr(&fromNode),
			After:     cloneNodePtr(&after),
			CreatedAt: time.Now().UTC(),
		})
	}
	return nil
}
