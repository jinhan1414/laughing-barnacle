package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"laughing-barnacle/internal/llm"
)

type SegmentExtraction struct {
	ProjectSummary string                       `json:"project_summary"`
	ProjectFacts   []string                     `json:"project_facts"`
	Candidates     []SegmentExtractionCandidate `json:"candidates"`
}

type SegmentExtractionCandidate struct {
	Name        string    `json:"name"`
	TargetPath  string    `json:"target_path"`
	TargetTitle string    `json:"target_title"`
	SchemaKind  string    `json:"schema_kind"`
	Summary     string    `json:"summary"`
	Facts       []string  `json:"facts"`
	Sections    []Section `json:"sections"`
	Confidence  float64   `json:"confidence"`
}

type SegmentExtractor interface {
	ExtractSegment(ctx context.Context, seg Segment) (SegmentExtraction, error)
}

type LLMSegmentExtractor struct {
	client      llm.Client
	model       string
	temperature float64
	maxTurns    int
}

func NewLLMSegmentExtractor(client llm.Client, model string, temperature float64) *LLMSegmentExtractor {
	return &LLMSegmentExtractor{
		client:      client,
		model:       strings.TrimSpace(model),
		temperature: temperature,
		maxTurns:    10,
	}
}

func (e *LLMSegmentExtractor) ExtractSegment(ctx context.Context, seg Segment) (SegmentExtraction, error) {
	if e == nil || e.client == nil {
		return SegmentExtraction{}, fmt.Errorf("llm extractor not initialized")
	}
	if strings.TrimSpace(e.model) == "" {
		return SegmentExtraction{}, fmt.Errorf("llm extractor model is required")
	}

	resp, err := e.client.Chat(ctx, llm.ChatRequest{
		Purpose: "memory_segment_extract",
		Model:   e.model,
		Messages: []llm.Message{
			{Role: "system", Content: strings.TrimSpace(`你是数字分身的记忆结构化提取器。
目标：把一个会话 segment 提取为结构化记忆 JSON。
严格要求：
1) 仅输出 JSON 对象，不要 markdown，不要解释。
2) 只提炼用户长期有价值信息。
3) 若信息不确定，保持低 confidence（0.30~0.55）并保留在 candidates。
4) target_path 仅允许以下前缀：/profile /preferences /constraints /goals。
5) sections 最多 2 段，每段 content 不超过 600 字。`)},
			{Role: "user", Content: buildSegmentExtractionPrompt(seg, e.maxTurns)},
		},
		Temperature: e.temperature,
	})
	if err != nil {
		return SegmentExtraction{}, fmt.Errorf("llm extract failed: %w", err)
	}

	extracted, err := parseSegmentExtraction(resp.Content)
	if err != nil {
		return SegmentExtraction{}, err
	}
	normalized, err := normalizeSegmentExtraction(extracted)
	if err != nil {
		return SegmentExtraction{}, err
	}
	return normalized, nil
}

func buildRuleBasedSegmentExtraction(seg Segment) SegmentExtraction {
	return SegmentExtraction{
		ProjectSummary: buildSegmentProjectSummary(seg),
		ProjectFacts:   buildSegmentProjectFacts(seg),
		Candidates:     buildSegmentCandidates(seg),
	}
}

func buildSegmentExtractionPrompt(seg Segment, maxTurns int) string {
	if maxTurns <= 0 {
		maxTurns = 10
	}
	turns := seg.Turns
	if len(turns) > maxTurns {
		turns = turns[len(turns)-maxTurns:]
	}
	type compactTurn struct {
		User      string `json:"user"`
		Assistant string `json:"assistant"`
	}
	input := struct {
		SegmentID   string        `json:"segment_id"`
		CloseReason string        `json:"close_reason,omitempty"`
		TurnCount   int           `json:"turn_count"`
		Turns       []compactTurn `json:"turns"`
	}{
		SegmentID:   strings.TrimSpace(seg.ID),
		CloseReason: strings.TrimSpace(seg.CloseReason),
		TurnCount:   len(seg.Turns),
		Turns:       make([]compactTurn, 0, len(turns)),
	}
	for _, turn := range turns {
		input.Turns = append(input.Turns, compactTurn{
			User:      trimRunes(turn.User, 320),
			Assistant: trimRunes(turn.Assistant, 320),
		})
	}
	data, _ := json.Marshal(input)
	return strings.TrimSpace(`请输出如下 JSON 结构：
{
  "project_summary": "string",
  "project_facts": ["string"],
  "candidates": [
    {
      "name": "profile|preferences|constraints|goals",
      "target_path": "/profile/... 或 /preferences/... 或 /constraints/... 或 /goals/...",
      "target_title": "string",
      "schema_kind": "string",
      "summary": "string",
      "facts": ["string"],
      "sections": [{"id":"s1","title":"string","digest":"string","content":"string"}],
      "confidence": 0.45
    }
  ]
}
输入 segment：` + string(data))
}

func parseSegmentExtraction(content string) (SegmentExtraction, error) {
	payload := strings.TrimSpace(content)
	if payload == "" {
		return SegmentExtraction{}, fmt.Errorf("empty extraction response")
	}
	payload = strings.TrimSpace(extractJSONObject(payload))
	if payload == "" {
		return SegmentExtraction{}, fmt.Errorf("invalid extraction response")
	}
	var extracted SegmentExtraction
	if err := json.Unmarshal([]byte(payload), &extracted); err != nil {
		return SegmentExtraction{}, fmt.Errorf("decode extraction response: %w", err)
	}
	return extracted, nil
}

func normalizeSegmentExtraction(extracted SegmentExtraction) (SegmentExtraction, error) {
	extracted.ProjectSummary = trimRunes(extracted.ProjectSummary, 200)
	extracted.ProjectFacts = normalizeStringList(extracted.ProjectFacts, 16, 120)

	out := make([]SegmentExtractionCandidate, 0, len(extracted.Candidates))
	for i, candidate := range extracted.Candidates {
		name := sanitizeCandidateName(candidate.Name, i+1)
		targetPath, err := normalizePath(candidate.TargetPath)
		if err != nil {
			continue
		}
		if !isAllowedCandidateTargetPath(targetPath) {
			continue
		}
		targetTitle := trimRunes(candidate.TargetTitle, 80)
		if targetTitle == "" {
			targetTitle = strings.ReplaceAll(name, "_", " ")
		}
		schemaKind := trimRunes(candidate.SchemaKind, 60)
		if schemaKind == "" {
			schemaKind = "generic"
		}
		summary := trimRunes(candidate.Summary, 220)
		if summary == "" {
			summary = "(无摘要)"
		}
		out = append(out, SegmentExtractionCandidate{
			Name:        name,
			TargetPath:  targetPath,
			TargetTitle: targetTitle,
			SchemaKind:  schemaKind,
			Summary:     summary,
			Facts:       normalizeStringList(candidate.Facts, 16, 120),
			Sections:    normalizeSections(limitSections(candidate.Sections, 2)),
			Confidence:  clamp(candidate.Confidence, 0.3, 0.95),
		})
	}
	extracted.Candidates = out
	if strings.TrimSpace(extracted.ProjectSummary) == "" && len(extracted.ProjectFacts) == 0 && len(extracted.Candidates) == 0 {
		return SegmentExtraction{}, fmt.Errorf("extraction is empty")
	}
	return extracted, nil
}

func isAllowedCandidateTargetPath(path string) bool {
	for _, prefix := range []string{"/profile", "/preferences", "/constraints", "/goals"} {
		if strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func sanitizeCandidateName(raw string, index int) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		raw = fmt.Sprintf("candidate_%d", index)
	}
	var b strings.Builder
	for _, ch := range raw {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			b.WriteRune(ch)
			continue
		}
		b.WriteRune('-')
	}
	name := strings.Trim(strings.Join(strings.Fields(strings.ReplaceAll(b.String(), "-", " ")), "-"), "-")
	if name == "" {
		return fmt.Sprintf("candidate_%d", index)
	}
	return name
}

func limitSections(in []Section, limit int) []Section {
	if len(in) == 0 || limit <= 0 {
		return nil
	}
	if len(in) > limit {
		in = in[:limit]
	}
	out := make([]Section, 0, len(in))
	for _, sec := range in {
		sec.Content = trimRunes(sec.Content, 600)
		sec.Digest = trimRunes(sec.Digest, 120)
		sec.Title = trimRunes(sec.Title, 80)
		if strings.TrimSpace(sec.Content) == "" {
			continue
		}
		out = append(out, sec)
	}
	return out
}

func extractJSONObject(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if strings.HasPrefix(input, "```") {
		input = strings.TrimPrefix(input, "```")
		input = strings.TrimSpace(input)
		if len(input) >= 4 && strings.EqualFold(input[:4], "json") {
			input = strings.TrimSpace(input[4:])
		}
		if idx := strings.LastIndex(input, "```"); idx >= 0 {
			input = strings.TrimSpace(input[:idx])
		}
	}
	start := strings.Index(input, "{")
	end := strings.LastIndex(input, "}")
	if start < 0 || end <= start {
		return ""
	}
	return input[start : end+1]
}
