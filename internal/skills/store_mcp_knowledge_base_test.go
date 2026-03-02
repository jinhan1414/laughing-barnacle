package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMCPKnowledgeBaseSkill_IsIndexedAndReadable(t *testing.T) {
	root := t.TempDir()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	builtinDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "builtin-skills")
	files := []string{
		"SKILL.md",
		filepath.Join("references", "authority.md"),
		filepath.Join("references", "concepts.md"),
		filepath.Join("references", "protocol-lifecycle-and-transport.md"),
		filepath.Join("references", "sdk-and-debugging.md"),
		filepath.Join("references", "repo-implementation.md"),
	}

	store, err := NewStoreWithBuiltinDir(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"), builtinDir)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	index := store.ListEnabledSkillIndex()
	found := ""
	for _, line := range index {
		if strings.Contains(line, "skill_id=mcp-knowledge-base") {
			found = line
			break
		}
	}
	if found == "" {
		t.Fatalf("expected mcp-knowledge-base in enabled skill index, got %v", index)
	}
	if !strings.Contains(found, "MCP 协议知识库") {
		t.Fatalf("expected indexed skill name in %q", found)
	}

	prompt, ok := store.ReadEnabledSkillPrompt("mcp-knowledge-base")
	if !ok {
		t.Fatalf("expected mcp-knowledge-base prompt readable")
	}

	requiredSnippets := []string{
		"https://modelcontextprotocol.io/specification/versioning",
		"https://modelcontextprotocol.io/specification/2025-11-25",
		"https://github.com/modelcontextprotocol",
		"references/authority.md",
		"references/concepts.md",
		"references/protocol-lifecycle-and-transport.md",
		"references/sdk-and-debugging.md",
		"references/repo-implementation.md",
		"Streamable HTTP",
		"Host / Client / Server",
		"/api/mcp/services/save|toggle|delete",
		"tools/list",
		"tools/call",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("expected prompt to contain %q, got %q", snippet, prompt)
		}
	}

	runtimeRoot := filepath.Join(root, "skills", "mcp-knowledge-base")
	for _, rel := range files {
		if _, err := os.Stat(filepath.Join(runtimeRoot, rel)); err != nil {
			t.Fatalf("expected synced skill file %s: %v", rel, err)
		}
	}
}
