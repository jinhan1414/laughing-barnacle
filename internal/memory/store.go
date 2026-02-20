package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"laughing-barnacle/internal/conversation"

	bolt "go.etcd.io/bbolt"
)

type NodeType string

const (
	NodeTypeDir  NodeType = "dir"
	NodeTypeFile NodeType = "file"
)

type SegmentStatus string

const (
	SegmentStatusOpen       SegmentStatus = "open"
	SegmentStatusClosed     SegmentStatus = "closed"
	SegmentStatusProcessing SegmentStatus = "processing"
	SegmentStatusPersisted  SegmentStatus = "persisted"
	SegmentStatusFailed     SegmentStatus = "failed"
)

const (
	bucketNodes    = "memory_nodes"
	bucketChildren = "memory_children"
	bucketSegments = "memory_segments"
	bucketMeta     = "memory_meta"
	bucketAudit    = "memory_audit"

	metaOpenSegmentID = "open_segment_id"
)

var (
	ErrNodeNotFound     = errors.New("memory node not found")
	ErrSectionNotFound  = errors.New("memory section not found")
	ErrPathConflict     = errors.New("memory path conflict")
	ErrRevisionConflict = errors.New("memory revision conflict")

	pathSegPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

type Ref struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type Section struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Digest  string `json:"digest"`
	Content string `json:"content"`
}

type FileContent struct {
	Summary  string    `json:"summary"`
	Facts    []string  `json:"facts,omitempty"`
	Sections []Section `json:"sections,omitempty"`
	Refs     []Ref     `json:"refs,omitempty"`
}

type Node struct {
	ID            string       `json:"id"`
	Path          string       `json:"path"`
	Title         string       `json:"title"`
	Type          NodeType     `json:"type"`
	SchemaKind    string       `json:"schema_kind"`
	SchemaVersion int          `json:"schema_version"`
	Tags          []string     `json:"tags,omitempty"`
	Source        string       `json:"source,omitempty"`
	Confidence    float64      `json:"confidence,omitempty"`
	Revision      int64        `json:"revision"`
	Content       *FileContent `json:"content,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

type IndexItem struct {
	Path      string    `json:"path"`
	Title     string    `json:"title"`
	Type      NodeType  `json:"type"`
	Summary   string    `json:"summary,omitempty"`
	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type UpsertRequest struct {
	Mode             string
	Path             string
	Title            string
	Type             NodeType
	SchemaKind       string
	SchemaVersion    int
	Tags             []string
	Source           string
	Confidence       float64
	Summary          string
	Facts            []string
	Sections         []Section
	Refs             []Ref
	ExpectedRevision int64
}

type SegmentTurn struct {
	User      string                  `json:"user"`
	Assistant string                  `json:"assistant"`
	ToolCalls []conversation.ToolCall `json:"tool_calls,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
}

type Segment struct {
	ID             string        `json:"id"`
	Status         SegmentStatus `json:"status"`
	RetryCount     int           `json:"retry_count,omitempty"`
	Turns          []SegmentTurn `json:"turns"`
	StartedAt      time.Time     `json:"started_at"`
	LastUserAt     time.Time     `json:"last_user_at"`
	LastActivityAt time.Time     `json:"last_activity_at"`
	ClosedAt       time.Time     `json:"closed_at,omitempty"`
	CloseReason    string        `json:"close_reason,omitempty"`
	PersistedPaths []string      `json:"persisted_paths,omitempty"`
	Error          string        `json:"error,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	db   *bolt.DB
}

func NewStoreWithFile(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("memory store path is required")
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open memory db: %w", err)
	}
	s := &Store{path: path, db: db}
	if err := s.bootstrap(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *Store) bootstrap() error {
	if s.db == nil {
		return nil
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		for _, b := range []string{bucketNodes, bucketChildren, bucketSegments, bucketMeta, bucketAudit} {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("init memory db schema: %w", err)
	}
	for _, path := range []string{
		"/", "/meta", "/profile", "/preferences", "/constraints", "/goals", "/projects", "/routines", "/conversation", "/conversation/archive", "/inbox", "/inbox/pending", "/inbox/reviewed", "/inbox/trash",
	} {
		if _, err := s.UpsertNode(UpsertRequest{
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

func (s *Store) readNodeLocked(path string) (Node, bool, error) {
	var node Node
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNodes))
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(path))
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &node)
	})
	if err != nil {
		return Node{}, false, err
	}
	if strings.TrimSpace(node.Path) == "" {
		return Node{}, false, nil
	}
	return node, true, nil
}

func (s *Store) writeNodeLocked(node Node) error {
	data, err := json.Marshal(node)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNodes))
		if b == nil {
			return fmt.Errorf("memory nodes bucket missing")
		}
		return b.Put([]byte(node.Path), data)
	})
}

func (s *Store) deleteNodeKeyLocked(path string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNodes))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(path))
	})
}

func (s *Store) readChildrenLocked(path string) ([]string, error) {
	var out []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChildren))
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(path))
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &out)
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) writeChildrenLocked(path string, children []string) error {
	children = normalizePathList(children)
	data, err := json.Marshal(children)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChildren))
		if b == nil {
			return fmt.Errorf("memory children bucket missing")
		}
		return b.Put([]byte(path), data)
	})
}

func (s *Store) deleteChildrenKeyLocked(path string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChildren))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(path))
	})
}

func (s *Store) addChildLocked(dirPath, childPath string) error {
	children, err := s.readChildrenLocked(dirPath)
	if err != nil {
		return err
	}
	children = append(children, childPath)
	return s.writeChildrenLocked(dirPath, children)
}

func (s *Store) removeChildLocked(dirPath, childPath string) error {
	children, err := s.readChildrenLocked(dirPath)
	if err != nil {
		return err
	}
	out := make([]string, 0, len(children))
	for _, path := range children {
		if path != childPath {
			out = append(out, path)
		}
	}
	return s.writeChildrenLocked(dirPath, out)
}

func (s *Store) readSegmentLocked(id string) (Segment, bool, error) {
	var seg Segment
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSegments))
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(id))
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &seg)
	})
	if err != nil {
		return Segment{}, false, err
	}
	if strings.TrimSpace(seg.ID) == "" {
		return Segment{}, false, nil
	}
	return seg, true, nil
}

func (s *Store) writeSegmentLocked(seg Segment) error {
	data, err := json.Marshal(seg)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSegments))
		if b == nil {
			return fmt.Errorf("memory segments bucket missing")
		}
		return b.Put([]byte(seg.ID), data)
	})
}

func (s *Store) listSegmentsLocked() ([]Segment, error) {
	out := make([]Segment, 0, 16)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSegments))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var seg Segment
			if err := json.Unmarshal(v, &seg); err != nil {
				return nil
			}
			out = append(out, seg)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.Before(out[j].UpdatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) readMetaStringLocked(key string) (string, error) {
	var value string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMeta))
		if b == nil {
			return nil
		}
		value = strings.TrimSpace(string(b.Get([]byte(key))))
		return nil
	})
	return value, err
}

func (s *Store) writeMetaStringLocked(key, value string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMeta))
		if b == nil {
			return fmt.Errorf("memory meta bucket missing")
		}
		return b.Put([]byte(key), []byte(strings.TrimSpace(value)))
	})
}

func normalizePath(path string) (string, error) {
	path = strings.TrimSpace(strings.ToLower(path))
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("path must start with /")
	}
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	if path == "/" {
		return path, nil
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, part := range parts {
		if !pathSegPattern.MatchString(part) {
			return "", fmt.Errorf("invalid path segment: %s", part)
		}
	}
	return path, nil
}

func parentPath(path string) string {
	if path == "/" {
		return ""
	}
	idx := strings.LastIndex(path, "/")
	if idx <= 0 {
		return "/"
	}
	return path[:idx]
}

func pathTitle(path string) string {
	if path == "/" {
		return "root"
	}
	seg := path[strings.LastIndex(path, "/")+1:]
	if seg == "" {
		return "root"
	}
	return strings.ReplaceAll(seg, "-", " ")
}

func buildNodeID(path string, now time.Time) string {
	return fmt.Sprintf("mem-%s-%d", strings.TrimPrefix(strings.ReplaceAll(path, "/", "-"), "-"), now.UnixNano())
}

func buildSegmentID(now time.Time) string {
	return fmt.Sprintf("seg-%s-%d", now.UTC().Format("20060102-150405"), now.UTC().UnixNano())
}

func normalizeStringList(in []string, limit int, maxRunes int) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := trimRunes(raw, maxRunes)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeSections(in []Section) []Section {
	if len(in) == 0 {
		return nil
	}
	out := make([]Section, 0, len(in))
	for i, sec := range in {
		id := strings.TrimSpace(sec.ID)
		if id == "" {
			id = fmt.Sprintf("s%d", i+1)
		}
		title := trimRunes(sec.Title, 80)
		if title == "" {
			title = fmt.Sprintf("section %d", i+1)
		}
		digest := trimRunes(sec.Digest, 120)
		content := trimRunes(sec.Content, 2000)
		if content == "" {
			continue
		}
		if digest == "" {
			digest = trimRunes(content, 80)
		}
		out = append(out, Section{ID: id, Title: title, Digest: digest, Content: content})
	}
	return out
}

func normalizeRefs(in []Ref) []Ref {
	if len(in) == 0 {
		return nil
	}
	out := make([]Ref, 0, len(in))
	for _, ref := range in {
		kind := trimRunes(ref.Kind, 40)
		value := trimRunes(ref.Value, 200)
		if kind == "" || value == "" {
			continue
		}
		out = append(out, Ref{Kind: kind, Value: value})
	}
	return out
}

func normalizePathList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, path := range in {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func normalizeToolCalls(in []conversation.ToolCall) []conversation.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]conversation.ToolCall, 0, len(in))
	for _, call := range in {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		call.Name = name
		call.Arguments = trimRunes(call.Arguments, 400)
		call.Result = trimRunes(call.Result, 400)
		call.Error = trimRunes(call.Error, 200)
		out = append(out, call)
	}
	return out
}

func trimRunes(input string, max int) string {
	input = strings.TrimSpace(input)
	if max <= 0 || input == "" {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return strings.TrimSpace(string(runes[:max-3])) + "..."
}

func clamp(value, minV, maxV float64) float64 {
	if value < minV {
		return minV
	}
	if value > maxV {
		return maxV
	}
	return value
}

func encodePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/")
	path = strings.ReplaceAll(path, "/", "-")
	if path == "" {
		return "root"
	}
	return path
}
