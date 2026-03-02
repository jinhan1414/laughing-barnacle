package skills

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadEnabledSkillResourceIndex_IncludesReferencesAndScripts(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	writeTestSkillFiles(t, skillsDir, "fixture-skill")

	store, err := NewStore(skillsDir, filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	index, err := store.ReadEnabledSkillResourceIndex("fixture-skill")
	if err != nil {
		t.Fatalf("ReadEnabledSkillResourceIndex error: %v", err)
	}
	if index.ID != "fixture-skill" {
		t.Fatalf("unexpected index id: %+v", index)
	}
	assertSkillResourcePresent(t, index.Resources, "SKILL.md", "skill", true, false)
	assertSkillResourcePresent(t, index.Resources, "references/guide.md", "reference", true, false)
	assertSkillResourcePresent(t, index.Resources, "scripts/check.ps1", "script", false, true)
}

func TestReadEnabledSkillResource_ReadsReferenceContent(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	writeTestSkillFiles(t, skillsDir, "fixture-skill")

	store, err := NewStore(skillsDir, filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	content, err := store.ReadEnabledSkillResource("fixture-skill", "references/guide.md")
	if err != nil {
		t.Fatalf("ReadEnabledSkillResource error: %v", err)
	}
	if content != "# Guide\n\nRead me." {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestReadEnabledSkillResource_RejectsInvalidPath(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	writeTestSkillFiles(t, skillsDir, "fixture-skill")

	store, err := NewStore(skillsDir, filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	_, err = store.ReadEnabledSkillResource("fixture-skill", "../secrets.txt")
	if !errors.Is(err, ErrInvalidSkillResourcePath) {
		t.Fatalf("expected invalid path error, got %v", err)
	}
}

func writeTestSkillFiles(t *testing.T, skillsDir, skillID string) {
	t.Helper()
	root := filepath.Join(skillsDir, skillID)
	mustWriteFile(t, filepath.Join(root, "SKILL.md"), "---\nname: \"Fixture\"\ndescription: \"Fixture skill\"\n---\n\nUse fixture skill.\n")
	mustWriteFile(t, filepath.Join(root, "references", "guide.md"), "# Guide\n\nRead me.\n")
	mustWriteFile(t, filepath.Join(root, "scripts", "check.ps1"), "Write-Output 'ok'\n")
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
}

func assertSkillResourcePresent(t *testing.T, resources []SkillResource, path, kind string, readable, executable bool) {
	t.Helper()
	for _, item := range resources {
		if item.Path == path && item.Kind == kind && item.Readable == readable && item.Executable == executable {
			return
		}
	}
	t.Fatalf("expected resource path=%s kind=%s readable=%t executable=%t, got %+v", path, kind, readable, executable, resources)
}
