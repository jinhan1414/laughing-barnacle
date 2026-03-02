package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"laughing-barnacle/internal/skills"
)

func TestHandleAPISkillSave_SavesSkillPackageResources(t *testing.T) {
	root := t.TempDir()
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	s := &Server{skillStore: skillStore}

	body := `{
		"id":"fixture-skill",
		"name":"Fixture",
		"description":"Fixture skill",
		"prompt":"Use fixture skill.",
		"enabled":true,
		"resources":[
			{"path":"references/guide.md","content":"# Guide\n\nRead me.\n"},
			{"path":"scripts/check.ps1","content":"Write-Output 'ok'\n"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/skills/save", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleAPISkillSave(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleAPISkillSave expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	index, err := skillStore.ReadEnabledSkillResourceIndex("fixture-skill")
	if err != nil {
		t.Fatalf("ReadEnabledSkillResourceIndex error: %v", err)
	}
	if len(index.Resources) < 3 {
		t.Fatalf("expected saved resources, got %+v", index.Resources)
	}
}

func TestHandleAPISkillSave_RejectsInvalidResourcePath(t *testing.T) {
	root := t.TempDir()
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	s := &Server{skillStore: skillStore}

	body := `{
		"id":"fixture-skill",
		"name":"Fixture",
		"description":"Fixture skill",
		"prompt":"Use fixture skill.",
		"enabled":true,
		"resources":[
			{"path":"../secret.txt","content":"nope"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/skills/save", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleAPISkillSave(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid skill resource path") {
		t.Fatalf("unexpected error body=%s", rec.Body.String())
	}
}
