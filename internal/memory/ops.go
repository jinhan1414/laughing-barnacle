package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

type MaintenanceReport struct {
	RanAt             time.Time `json:"ran_at"`
	RetriedSegments   int       `json:"retried_segments"`
	RemovedTrashNodes int       `json:"removed_trash_nodes"`
	RepairedChildren  int       `json:"repaired_children"`
}

type AuditEntry struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	Path      string    `json:"path,omitempty"`
	FromPath  string    `json:"from_path,omitempty"`
	ToPath    string    `json:"to_path,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Before    *Node     `json:"before,omitempty"`
	After     *Node     `json:"after,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) persistSegmentStructuredMemoryLocked(seg Segment) ([]string, error) {
	if strings.TrimSpace(seg.ID) == "" {
		return nil, fmt.Errorf("segment id is required")
	}
	extracted, err := s.extractSegmentLocked(seg)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, 8)

	projectPath := "/projects/session-journal/" + seg.ID
	projectNode, err := s.upsertNodeLocked(UpsertRequest{
		Mode:          "replace",
		Path:          projectPath,
		Type:          NodeTypeFile,
		Title:         "会话项目记录 " + seg.ID,
		SchemaKind:    "project_journal",
		SchemaVersion: 1,
		Source:        "chat",
		Confidence:    0.88,
		Summary:       extracted.ProjectSummary,
		Facts:         extracted.ProjectFacts,
		Sections:      buildArchiveSections(seg.Turns),
		Refs: []Ref{
			{Kind: "segment_id", Value: seg.ID},
			{Kind: "close_reason", Value: seg.CloseReason},
		},
	})
	if err != nil {
		return nil, err
	}
	paths = append(paths, projectNode.Path)

	for _, candidate := range extracted.Candidates {
		pendingPath := "/inbox/pending/" + seg.ID + "-" + candidate.Name
		refs := []Ref{
			{Kind: "target_path", Value: candidate.TargetPath},
			{Kind: "target_title", Value: candidate.TargetTitle},
			{Kind: "target_schema_kind", Value: candidate.SchemaKind},
			{Kind: "target_confidence", Value: fmt.Sprintf("%.2f", candidate.Confidence)},
			{Kind: "segment_id", Value: seg.ID},
		}
		node, upsertErr := s.upsertNodeLocked(UpsertRequest{
			Mode:          "replace",
			Path:          pendingPath,
			Type:          NodeTypeFile,
			Title:         "待审核 " + candidate.TargetTitle,
			SchemaKind:    "memory_candidate",
			SchemaVersion: 1,
			Source:        "chat",
			Confidence:    candidate.Confidence,
			Summary:       candidate.Summary,
			Facts:         candidate.Facts,
			Sections:      candidate.Sections,
			Refs:          refs,
		})
		if upsertErr != nil {
			return nil, upsertErr
		}
		paths = append(paths, node.Path)
	}
	return paths, nil
}

func buildSegmentProjectSummary(seg Segment) string {
	firstUser, _, _, _, _, _, _ := segmentStats(seg)
	if strings.TrimSpace(firstUser) == "" {
		return fmt.Sprintf("会话片段 %s 已沉淀，共 %d 轮。", seg.ID, len(seg.Turns))
	}
	return fmt.Sprintf("会话片段 %s：围绕“%s”展开，共 %d 轮。", seg.ID, trimRunes(firstUser, 32), len(seg.Turns))
}

func buildSegmentProjectFacts(seg Segment) []string {
	_, _, _, userTurns, assistantTurns, toolCalls, toolErrors := segmentStats(seg)
	facts := []string{
		"segment_id=" + strings.TrimSpace(seg.ID),
		fmt.Sprintf("turns=%d", len(seg.Turns)),
		fmt.Sprintf("user_turns=%d", userTurns),
		fmt.Sprintf("assistant_turns=%d", assistantTurns),
		fmt.Sprintf("tool_calls=%d", toolCalls),
		fmt.Sprintf("tool_errors=%d", toolErrors),
	}
	if strings.TrimSpace(seg.CloseReason) != "" {
		facts = append(facts, "close_reason="+strings.TrimSpace(seg.CloseReason))
	}
	return facts
}

func buildSegmentCandidates(seg Segment) []SegmentExtractionCandidate {
	firstUser, lastUser, lastAssistant, userTurns, assistantTurns, toolCalls, toolErrors := segmentStats(seg)
	avgUserLen := 0
	if userTurns > 0 {
		total := 0
		for _, turn := range seg.Turns {
			total += len([]rune(strings.TrimSpace(turn.User)))
		}
		avgUserLen = total / userTurns
	}

	goalSummary := "本段用户目标待确认"
	if strings.TrimSpace(firstUser) != "" {
		goalSummary = "本段用户目标候选：" + trimRunes(firstUser, 52)
	}

	preferenceSummary := "用户偏好候选：回答粒度待确认"
	if avgUserLen > 0 {
		switch {
		case avgUserLen <= 24:
			preferenceSummary = "用户偏好候选：倾向简短交互"
		case avgUserLen >= 120:
			preferenceSummary = "用户偏好候选：倾向详细讨论"
		default:
			preferenceSummary = "用户偏好候选：倾向中等粒度回答"
		}
	}

	constraintSummary := "约束候选：本段未观测到新增运行约束"
	if toolErrors > 0 {
		constraintSummary = "约束候选：本段存在工具失败，需关注运行约束"
	}

	profileSummary := "用户画像候选：补充近期表达样本"
	if strings.TrimSpace(lastUser) != "" {
		profileSummary = "用户画像候选：近期表达“" + trimRunes(lastUser, 42) + "”"
	}

	baseSection := func(id, title, content string) []Section {
		content = strings.TrimSpace(content)
		if content == "" {
			return nil
		}
		return []Section{{
			ID:      id,
			Title:   title,
			Digest:  trimRunes(content, 48),
			Content: trimRunes(content, 1200),
		}}
	}

	return []SegmentExtractionCandidate{
		{
			Name:        "goals",
			TargetPath:  "/goals/active/" + seg.ID,
			TargetTitle: "目标沉淀 " + seg.ID,
			SchemaKind:  "goal",
			Summary:     goalSummary,
			Facts: []string{
				"segment_id=" + seg.ID,
				fmt.Sprintf("user_turns=%d", userTurns),
				"source=memory_candidate",
			},
			Sections:   baseSection("s1", "目标线索", firstUser+"\n"+lastAssistant),
			Confidence: 0.45,
		},
		{
			Name:        "preferences",
			TargetPath:  "/preferences/interaction/" + seg.ID,
			TargetTitle: "偏好沉淀 " + seg.ID,
			SchemaKind:  "preference",
			Summary:     preferenceSummary,
			Facts: []string{
				"segment_id=" + seg.ID,
				fmt.Sprintf("avg_user_len=%d", avgUserLen),
				fmt.Sprintf("assistant_turns=%d", assistantTurns),
				"source=memory_candidate",
			},
			Sections:   baseSection("s1", "互动样本", firstUser+"\n"+lastAssistant),
			Confidence: 0.42,
		},
		{
			Name:        "constraints",
			TargetPath:  "/constraints/runtime/" + seg.ID,
			TargetTitle: "约束沉淀 " + seg.ID,
			SchemaKind:  "constraint",
			Summary:     constraintSummary,
			Facts: []string{
				"segment_id=" + seg.ID,
				fmt.Sprintf("tool_calls=%d", toolCalls),
				fmt.Sprintf("tool_errors=%d", toolErrors),
				"source=memory_candidate",
			},
			Sections:   baseSection("s1", "运行上下文", lastAssistant),
			Confidence: 0.5,
		},
		{
			Name:        "profile",
			TargetPath:  "/profile/session/" + seg.ID,
			TargetTitle: "画像沉淀 " + seg.ID,
			SchemaKind:  "profile_note",
			Summary:     profileSummary,
			Facts: []string{
				"segment_id=" + seg.ID,
				fmt.Sprintf("turns=%d", len(seg.Turns)),
				"source=memory_candidate",
			},
			Sections:   baseSection("s1", "表达片段", firstUser+"\n"+lastUser),
			Confidence: 0.4,
		},
	}
}

func segmentStats(seg Segment) (firstUser, lastUser, lastAssistant string, userTurns, assistantTurns, toolCalls, toolErrors int) {
	for _, turn := range seg.Turns {
		user := strings.TrimSpace(turn.User)
		assistant := strings.TrimSpace(turn.Assistant)
		if user != "" {
			userTurns++
			if firstUser == "" {
				firstUser = user
			}
			lastUser = user
		}
		if assistant != "" {
			assistantTurns++
			lastAssistant = assistant
		}
		for _, call := range turn.ToolCalls {
			toolCalls++
			if strings.TrimSpace(call.Error) != "" {
				toolErrors++
			}
		}
	}
	return firstUser, lastUser, lastAssistant, userTurns, assistantTurns, toolCalls, toolErrors
}

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
