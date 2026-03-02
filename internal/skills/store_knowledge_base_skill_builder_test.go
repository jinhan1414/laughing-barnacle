package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestKnowledgeBaseSkillBuilder_IsIndexedAndReadable(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "knowledge-base-skill-builder")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	sourceRoot := filepath.Join(filepath.Dir(currentFile), "..", "..", "data", "skills", "knowledge-base-skill-builder")
	files := []string{
		"SKILL.md",
		filepath.Join("references", "authority-and-scope.md"),
		filepath.Join("references", "structure-and-progressive-disclosure.md"),
		filepath.Join("references", "source-selection-and-writing.md"),
		filepath.Join("references", "knowledge-base-boundaries.md"),
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
		if strings.Contains(line, "skill_id=knowledge-base-skill-builder") {
			found = line
			break
		}
	}
	if found == "" {
		t.Fatalf("expected knowledge-base-skill-builder in enabled skill index, got %v", index)
	}
	if !strings.Contains(found, "知识库 Skill 制作") {
		t.Fatalf("expected indexed skill name in %q", found)
	}

	prompt, ok := store.ReadEnabledSkillPrompt("knowledge-base-skill-builder")
	if !ok {
		t.Fatalf("expected knowledge-base-skill-builder prompt readable")
	}

	requiredSnippets := []string{
		"https://www.anthropic.com/news/skills",
		"https://www.anthropic.com/engineering/agent-skills",
		"https://agentskills.io/",
		"https://platform.openai.com/docs/guides/retrieval",
		"https://platform.openai.com/docs/guides/tools-file-search",
		"https://platform.openai.com/docs/mcp/",
		"references/authority-and-scope.md",
		"references/structure-and-progressive-disclosure.md",
		"references/source-selection-and-writing.md",
		"references/knowledge-base-boundaries.md",
		"references/repo-implementation.md",
		"context__read(resource=\"skills\", action=\"read\", id=\"<skill_id>\", path=\"references/<file>.md\")",
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
