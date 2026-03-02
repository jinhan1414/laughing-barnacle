package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewStoreWithBuiltinDir_SyncsBuiltinSkillResources(t *testing.T) {
	root := t.TempDir()
	builtinDir := filepath.Join(root, "builtin-skills")
	writeBuiltinSkillFile(t, builtinDir, "demo-builtin", "SKILL.md", "---\nname: \"Demo Builtin\"\ndescription: \"Demo builtin skill\"\n---\n\nUse demo builtin.\n")
	writeBuiltinSkillFile(t, builtinDir, "demo-builtin", filepath.Join("references", "guide.md"), "# Guide\n\nRead me.\n")
	writeBuiltinSkillFile(t, builtinDir, "demo-builtin", filepath.Join("scripts", "check.ps1"), "Write-Output 'ok'\n")

	store, err := NewStoreWithBuiltinDir(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"), builtinDir)
	if err != nil {
		t.Fatalf("NewStoreWithBuiltinDir error: %v", err)
	}

	prompt, ok := store.ReadEnabledSkillPrompt("demo-builtin")
	if !ok {
		t.Fatalf("expected builtin prompt readable")
	}
	if !strings.Contains(prompt, "Use demo builtin.") {
		t.Fatalf("unexpected builtin prompt: %q", prompt)
	}

	index, err := store.ReadEnabledSkillResourceIndex("demo-builtin")
	if err != nil {
		t.Fatalf("ReadEnabledSkillResourceIndex error: %v", err)
	}
	assertSkillResourcePresent(t, index.Resources, "SKILL.md", "skill", true, false)
	assertSkillResourcePresent(t, index.Resources, "references/guide.md", "reference", true, false)
	assertSkillResourcePresent(t, index.Resources, "scripts/check.ps1", "script", false, true)
}

func TestNewStoreWithBuiltinDir_RejectsMalformedBuiltinSkill(t *testing.T) {
	root := t.TempDir()
	builtinDir := filepath.Join(root, "builtin-skills")
	if err := os.MkdirAll(filepath.Join(builtinDir, "broken-skill"), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	_, err := NewStoreWithBuiltinDir(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"), builtinDir)
	if err == nil || !strings.Contains(err.Error(), "missing SKILL.md") {
		t.Fatalf("expected missing SKILL.md error, got %v", err)
	}
}

func TestNewStoreWithBuiltinDir_RefreshKeepsEnabledState(t *testing.T) {
	root := t.TempDir()
	builtinDir := filepath.Join(root, "builtin-skills")
	writeBuiltinSkillFile(t, builtinDir, "demo-builtin", "SKILL.md", "---\nname: \"Demo Builtin\"\ndescription: \"Demo builtin skill\"\n---\n\nVersion one.\n")

	skillsDir := filepath.Join(root, "skills")
	statePath := filepath.Join(root, "skills_state.json")
	store, err := NewStoreWithBuiltinDir(skillsDir, statePath, builtinDir)
	if err != nil {
		t.Fatalf("NewStoreWithBuiltinDir error: %v", err)
	}
	if err := store.SetSkillEnabled("demo-builtin", false); err != nil {
		t.Fatalf("SetSkillEnabled error: %v", err)
	}

	writeBuiltinSkillFile(t, builtinDir, "demo-builtin", "SKILL.md", "---\nname: \"Demo Builtin\"\ndescription: \"Demo builtin skill\"\n---\n\nVersion two.\n")

	reloaded, err := NewStoreWithBuiltinDir(skillsDir, statePath, builtinDir)
	if err != nil {
		t.Fatalf("NewStoreWithBuiltinDir reload error: %v", err)
	}
	if _, ok := reloaded.ReadEnabledSkillPrompt("demo-builtin"); ok {
		t.Fatalf("expected builtin skill remain disabled after refresh")
	}

	skills := reloaded.ListSkills()
	if len(skills) == 0 {
		t.Fatalf("expected builtin skill listed")
	}
	found := false
	for _, skill := range skills {
		if skill.ID != "demo-builtin" {
			continue
		}
		found = true
		if skill.Enabled {
			t.Fatalf("expected demo-builtin disabled")
		}
		if !strings.Contains(skill.Prompt, "Version two.") {
			t.Fatalf("expected refreshed prompt, got %q", skill.Prompt)
		}
	}
	if !found {
		t.Fatalf("expected demo-builtin in skill list")
	}
}

func writeBuiltinSkillFile(t *testing.T, builtinDir, skillID, relPath, content string) {
	t.Helper()
	path := filepath.Join(builtinDir, skillID, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
}
