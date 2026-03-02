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

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	builtinDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "builtin-skills")
	files := []string{
		"SKILL.md",
		filepath.Join("references", "authority-and-scope.md"),
		filepath.Join("references", "structure-and-progressive-disclosure.md"),
		filepath.Join("references", "source-selection-and-writing.md"),
		filepath.Join("references", "knowledge-base-boundaries.md"),
		filepath.Join("references", "repo-implementation.md"),
	}

	store, err := NewStoreWithBuiltinDir(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"), builtinDir)
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

	runtimeRoot := filepath.Join(root, "skills", "knowledge-base-skill-builder")
	for _, rel := range files {
		if _, err := os.Stat(filepath.Join(runtimeRoot, rel)); err != nil {
			t.Fatalf("expected synced skill file %s: %v", rel, err)
		}
	}
}
