package memory

import (
	"fmt"
	"strings"
	"time"
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
