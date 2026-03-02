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
	skillDir := filepath.Join(root, "skills", "a2a-knowledge-base")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	source := filepath.Join(filepath.Dir(currentFile), "..", "..", "data", "skills", "a2a-knowledge-base", "SKILL.md")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), data, 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
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
		filepath.Join(filepath.Dir(source), "references", "authority.md"),
		filepath.Join(filepath.Dir(source), "references", "concepts.md"),
		filepath.Join(filepath.Dir(source), "references", "sdk-and-integration.md"),
		filepath.Join(filepath.Dir(source), "references", "repo-implementation.md"),
		filepath.Join(filepath.Dir(source), "references", "troubleshooting.md"),
	}
	for _, path := range requiredFiles {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected reference file %s: %v", path, err)
		}
	}
}
