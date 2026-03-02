package skills

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestUpsertSkillPackage_WritesResourcesAndReplacesOldFiles(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	err = store.UpsertSkillPackage(SkillPackage{
		Skill: Skill{
			ID:          "fixture-skill",
			Name:        "Fixture",
			Description: "Fixture skill",
			Prompt:      "Use fixture skill.",
			Enabled:     true,
		},
		Resources: []SkillPackageResource{
			{Path: "references/guide.md", Content: "# Guide\n\nRead me.\n"},
			{Path: "scripts/check.ps1", Content: "Write-Output 'ok'\n"},
		},
	})
	if err != nil {
		t.Fatalf("UpsertSkillPackage error: %v", err)
	}

	err = store.UpsertSkillPackage(SkillPackage{
		Skill: Skill{
			ID:          "fixture-skill",
			Name:        "Fixture",
			Description: "Fixture skill",
			Prompt:      "Use fixture skill v2.",
			Enabled:     true,
		},
		Resources: []SkillPackageResource{
			{Path: "references/guide.md", Content: "# Guide\n\nUpdated.\n"},
		},
	})
	if err != nil {
		t.Fatalf("second UpsertSkillPackage error: %v", err)
	}

	index, err := store.ReadEnabledSkillResourceIndex("fixture-skill")
	if err != nil {
		t.Fatalf("ReadEnabledSkillResourceIndex error: %v", err)
	}
	assertSkillResourcePresent(t, index.Resources, "references/guide.md", "reference", true, false)
	assertSkillResourceAbsent(t, index.Resources, "scripts/check.ps1")

	content, err := store.ReadEnabledSkillResource("fixture-skill", "references/guide.md")
	if err != nil {
		t.Fatalf("ReadEnabledSkillResource error: %v", err)
	}
	if content != "# Guide\n\nUpdated." {
		t.Fatalf("unexpected updated content: %q", content)
	}
}

func TestUpsertSkillPackage_RejectsInvalidResourcePath(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	err = store.UpsertSkillPackage(SkillPackage{
		Skill: Skill{
			ID:          "fixture-skill",
			Name:        "Fixture",
			Description: "Fixture skill",
			Prompt:      "Use fixture skill.",
			Enabled:     true,
		},
		Resources: []SkillPackageResource{
			{Path: "../secret.txt", Content: "nope"},
		},
	})
	if !errors.Is(err, ErrInvalidSkillResourcePath) {
		t.Fatalf("expected invalid resource path error, got %v", err)
	}
}

func assertSkillResourceAbsent(t *testing.T, resources []SkillResource, path string) {
	t.Helper()
	for _, item := range resources {
		if item.Path == path {
			t.Fatalf("expected resource %s absent, got %+v", path, resources)
		}
	}
}
