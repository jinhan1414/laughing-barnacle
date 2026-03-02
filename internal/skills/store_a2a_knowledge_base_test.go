package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestA2AKnowledgeBaseSkill_IsIndexedAndReadable(t *testing.T) {
	root := t.TempDir()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	builtinDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "builtin-skills")
	sourceRoot := filepath.Join(builtinDir, "a2a-knowledge-base")

	store, err := NewStoreWithBuiltinDir(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"), builtinDir)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	index := store.ListEnabledSkillIndex()
	found := ""
	for _, line := range index {
		if strings.Contains(line, "skill_id=a2a-knowledge-base") {
			found = line
			break
		}
	}
	if found == "" {
		t.Fatalf("expected a2a-knowledge-base in enabled skill index, got %v", index)
	}
	if !strings.Contains(found, "A2A 协议知识库") {
		t.Fatalf("expected indexed skill name in %q", found)
	}

	prompt, ok := store.ReadEnabledSkillPrompt("a2a-knowledge-base")
	if !ok {
		t.Fatalf("expected a2a-knowledge-base prompt readable")
	}

	requiredSnippets := []string{
		"https://a2a-protocol.org/latest/",
		"https://github.com/a2aproject/A2A",
		"https://github.com/a2aproject/a2a-go",
		"https://github.com/a2aproject/a2a-python",
		"references/authority.md",
		"references/concepts.md",
		"references/sdk-and-integration.md",
		"references/repo-implementation.md",
		"references/troubleshooting.md",
		"Agent Card",
		"Task",
		"MCP",
		"integrations/codex-a2a/README.md",
		"internal/a2a/provider_sdk.go",
		"/api/a2a/agents/save|toggle|delete",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("expected prompt to contain %q, got %q", snippet, prompt)
		}
	}

	requiredFiles := []string{
		filepath.Join(sourceRoot, "SKILL.md"),
		filepath.Join(sourceRoot, "references", "authority.md"),
		filepath.Join(sourceRoot, "references", "concepts.md"),
		filepath.Join(sourceRoot, "references", "sdk-and-integration.md"),
		filepath.Join(sourceRoot, "references", "repo-implementation.md"),
		filepath.Join(sourceRoot, "references", "troubleshooting.md"),
	}
	for _, path := range requiredFiles {
		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			t.Fatalf("Rel error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, "skills", "a2a-knowledge-base", rel)); err != nil {
			t.Fatalf("expected synced skill file %s: %v", rel, err)
		}
	}
}
