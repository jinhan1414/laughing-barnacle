package memory

import (
	"encoding/json"
	"fmt"
	"laughing-barnacle/internal/conversation"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

func (s *Store) ListNodes(limit int) []Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Node, 0, 64)
	_ = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNodes))
		if b == nil {
			return nil
		}
		_ = b.ForEach(func(_, v []byte) error {
			var node Node
			if err := json.Unmarshal(v, &node); err != nil {
				return nil
			}
			out = append(out, node)
			return nil
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].Path < out[j].Path
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *Store) ListIndexLines(limit int) []string {
	nodes := s.ListNodes(0)
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node.Type != NodeTypeFile || node.Content == nil {
			continue
		}
		if !strings.HasPrefix(node.Path, "/profile") && !strings.HasPrefix(node.Path, "/preferences") && !strings.HasPrefix(node.Path, "/constraints") && !strings.HasPrefix(node.Path, "/goals") && !strings.HasPrefix(node.Path, "/projects") {
			continue
		}
		line := fmt.Sprintf("path=%s | title=%s | summary=%s | rev=%d", node.Path, trimRunes(node.Title, 24), trimRunes(node.Content.Summary, 36), node.Revision)
		out = append(out, line)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Store) AppendTurn(user, assistant string, toolCalls []conversation.ToolCall, now time.Time) error {
	user = strings.TrimSpace(user)
	assistant = strings.TrimSpace(assistant)
	if user == "" && assistant == "" {
		return fmt.Errorf("empty turn")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	segID, err := s.readMetaStringLocked(metaOpenSegmentID)
	if err != nil {
		return err
	}
	var seg Segment
	if strings.TrimSpace(segID) == "" {
		seg = Segment{ID: buildSegmentID(now), Status: SegmentStatusOpen, StartedAt: now, LastUserAt: now, LastActivityAt: now, CreatedAt: now, UpdatedAt: now}
	} else {
		existing, found, err := s.readSegmentLocked(segID)
		if err != nil {
			return err
		}
		if !found || existing.Status != SegmentStatusOpen {
			seg = Segment{ID: buildSegmentID(now), Status: SegmentStatusOpen, StartedAt: now, LastUserAt: now, LastActivityAt: now, CreatedAt: now, UpdatedAt: now}
		} else {
			seg = existing
		}
	}
	seg.Turns = append(seg.Turns, SegmentTurn{User: trimRunes(user, 1800), Assistant: trimRunes(assistant, 1800), ToolCalls: normalizeToolCalls(toolCalls), CreatedAt: now})
	seg.LastUserAt = now
	seg.LastActivityAt = now
	seg.UpdatedAt = now
	if err := s.writeSegmentLocked(seg); err != nil {
		return err
	}
	if err := s.writeMetaStringLocked(metaOpenSegmentID, seg.ID); err != nil {
		return err
	}
	return nil
}

func (s *Store) ListSegments(limit int) []Segment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Segment, 0, 16)
	_ = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSegments))
		if b == nil {
			return nil
		}
		_ = b.ForEach(func(_, v []byte) error {
			var seg Segment
			if err := json.Unmarshal(v, &seg); err != nil {
				return nil
			}
			out = append(out, seg)
			return nil
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ID < out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *Store) CloseIdleSegments(now time.Time, idleWindow, maxWindow time.Duration, maxMessages int) ([]Segment, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	closed := make([]Segment, 0, 4)
	segments, err := s.listSegmentsLocked()
	if err != nil {
		return nil, err
	}
	for _, seg := range segments {
		if seg.Status != SegmentStatusOpen {
			continue
		}
		shouldClose := false
		reason := ""
		if idleWindow > 0 && !seg.LastUserAt.IsZero() && now.Sub(seg.LastUserAt) >= idleWindow {
			shouldClose = true
			reason = "idle_timeout"
		}
		if !shouldClose && maxWindow > 0 && !seg.StartedAt.IsZero() && now.Sub(seg.StartedAt) >= maxWindow {
			shouldClose = true
			reason = "max_window"
		}
		if !shouldClose && maxMessages > 0 && len(seg.Turns) >= maxMessages {
			shouldClose = true
			reason = "max_messages"
		}
		if !shouldClose {
			continue
		}
		seg.Status = SegmentStatusClosed
		seg.ClosedAt = now
		seg.CloseReason = reason
		seg.UpdatedAt = now
		if err := s.writeSegmentLocked(seg); err != nil {
			return nil, err
		}
		openID, _ := s.readMetaStringLocked(metaOpenSegmentID)
		if openID == seg.ID {
			_ = s.writeMetaStringLocked(metaOpenSegmentID, "")
		}
		closed = append(closed, seg)
	}
	return closed, nil
}

func (s *Store) ProcessClosedSegments(now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	segments, err := s.listSegmentsLocked()
	if err != nil {
		return err
	}
	for _, seg := range segments {
		if seg.Status != SegmentStatusClosed {
			continue
		}
		seg.Status = SegmentStatusProcessing
		seg.UpdatedAt = now
		if err := s.writeSegmentLocked(seg); err != nil {
			return err
		}
		paths, persistErr := s.persistSegmentToArchiveLocked(seg)
		if persistErr != nil {
			seg.Status = SegmentStatusFailed
			seg.Error = trimRunes(persistErr.Error(), 220)
			seg.UpdatedAt = now
			if err := s.writeSegmentLocked(seg); err != nil {
				return err
			}
			continue
		}
		extractedPaths, extractErr := s.persistSegmentStructuredMemoryLocked(seg)
		if extractErr != nil {
			seg.Status = SegmentStatusFailed
			seg.Error = trimRunes(extractErr.Error(), 220)
			seg.UpdatedAt = now
			if err := s.writeSegmentLocked(seg); err != nil {
				return err
			}
			continue
		}
		paths = append(paths, extractedPaths...)
		seg.Status = SegmentStatusPersisted
		seg.PersistedPaths = paths
		seg.Error = ""
		seg.UpdatedAt = now
		if err := s.writeSegmentLocked(seg); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) persistSegmentToArchiveLocked(seg Segment) ([]string, error) {
	archiveDir := "/conversation/archive/" + seg.ID
	if _, err := s.upsertNodeLocked(UpsertRequest{Mode: "patch", Path: archiveDir, Type: NodeTypeDir, Title: "会话归档 " + seg.ID, SchemaKind: "conversation_archive", SchemaVersion: 1, Source: "system", Confidence: 1}); err != nil {
		return nil, err
	}
	sections := buildArchiveSections(seg.Turns)
	summary := fmt.Sprintf("会话归档，共 %d 轮。", len(seg.Turns))
	node, err := s.upsertNodeLocked(UpsertRequest{Mode: "replace", Path: archiveDir + "/index", Type: NodeTypeFile, Title: "归档索引", SchemaKind: "archive_index", SchemaVersion: 1, Source: "system", Confidence: 1, Summary: summary, Sections: sections, Refs: []Ref{{Kind: "segment_id", Value: seg.ID}}})
	if err != nil {
		return nil, err
	}
	return []string{archiveDir, node.Path}, nil
}

func buildArchiveSections(turns []SegmentTurn) []Section {
	if len(turns) == 0 {
		return nil
	}
	sections := make([]Section, 0, (len(turns)+2)/3)
	for i := 0; i < len(turns); i += 3 {
		end := i + 3
		if end > len(turns) {
			end = len(turns)
		}
		chunk := turns[i:end]
		id := fmt.Sprintf("s%d", len(sections)+1)
		title := fmt.Sprintf("对话片段 %d", len(sections)+1)
		digest := trimRunes(chunk[0].User, 40)
		if strings.TrimSpace(digest) == "" {
			digest = trimRunes(chunk[0].Assistant, 40)
		}
		if strings.TrimSpace(digest) == "" {
			digest = "(无摘要)"
		}
		var b strings.Builder
		for idx, turn := range chunk {
			if strings.TrimSpace(turn.User) != "" {
				b.WriteString(fmt.Sprintf("%d. [user] %s\n", idx+1, trimRunes(turn.User, 240)))
			}
			if strings.TrimSpace(turn.Assistant) != "" {
				b.WriteString(fmt.Sprintf("%d. [assistant] %s\n", idx+1, trimRunes(turn.Assistant, 240)))
			}
		}
		sections = append(sections, Section{ID: id, Title: title, Digest: digest, Content: strings.TrimSpace(b.String())})
	}
	return sections
}
