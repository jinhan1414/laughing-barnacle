package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAgentSkillsKnowledgeBaseSkill_IsIndexedAndReadable(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "agent-skills-knowledge-base")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	sourceRoot := filepath.Join(filepath.Dir(currentFile), "..", "..", "data", "skills", "agent-skills-knowledge-base")
	files := []string{
		"SKILL.md",
		filepath.Join("references", "authority.md"),
		filepath.Join("references", "concepts-and-boundaries.md"),
		filepath.Join("references", "format-and-loading.md"),
		filepath.Join("references", "authoring-evaluation-and-safety.md"),
		filepath.Join("references", "repo-implementation.md"),
	}
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(sourceRoot, rel))
		if err != nil {
			t.Fatalf("ReadFile %s error: %v", rel, err)
		}
		target := filepath.Join(skillDir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("MkdirAll %s error: %v", rel, err)
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatalf("WriteFile %s error: %v", rel, err)
		}
	}

	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	index := store.ListEnabledSkillIndex()
	found := ""
	for _, line := range index {
		if strings.Contains(line, "skill_id=agent-skills-knowledge-base") {
			found = line
			break
		}
	}
	if found == "" {
		t.Fatalf("expected agent-skills-knowledge-base in enabled skill index, got %v", index)
	}
	if !strings.Contains(found, "Agent Skills 知识库") {
		t.Fatalf("expected indexed skill name in %q", found)
	}

	prompt, ok := store.ReadEnabledSkillPrompt("agent-skills-knowledge-base")
	if !ok {
		t.Fatalf("expected agent-skills-knowledge-base prompt readable")
	}

	requiredSnippets := []string{
		"https://www.anthropic.com/news/skills",
		"https://www.anthropic.com/engineering/agent-skills",
		"https://agentskills.io/",
		"https://openai.com/index/introducing-codex/",
		"https://platform.openai.com/docs/mcp",
		"references/authority.md",
		"references/concepts-and-boundaries.md",
		"references/format-and-loading.md",
		"references/authoring-evaluation-and-safety.md",
		"references/repo-implementation.md",
		"SKILL.md",
		"scripts/ + references/ + assets/",
		"context__read(resource=\\\"skills\\\", action=\\\"index|read\\\")",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("expected prompt to contain %q, got %q", snippet, prompt)
		}
	}

	for _, rel := range files[1:] {
		if _, err := os.Stat(filepath.Join(sourceRoot, rel)); err != nil {
			t.Fatalf("expected reference file %s: %v", rel, err)
		}
	}
}
