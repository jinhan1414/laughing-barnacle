package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"laughing-barnacle/internal/skills"
)

func TestHandleAPISkillIndexAndRead(t *testing.T) {
	root := t.TempDir()
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	writeSkillResourceFixture(t, filepath.Join(root, "skills"), "fixture-skill")
	s := &Server{skillStore: skillStore}

	indexReq := httptest.NewRequest(http.MethodGet, "/api/skills/index?id=fixture-skill", nil)
	indexRec := httptest.NewRecorder()
	s.handleAPISkillIndex(indexRec, indexReq)
	if indexRec.Code != http.StatusOK {
		t.Fatalf("handleAPISkillIndex expected 200, got %d body=%s", indexRec.Code, indexRec.Body.String())
	}
	var index skills.SkillResourceIndex
	if err := json.Unmarshal(indexRec.Body.Bytes(), &index); err != nil {
		t.Fatalf("decode index error: %v", err)
	}
	if len(index.Resources) < 3 {
		t.Fatalf("expected resource index entries, got %+v", index)
	}

	readReq := httptest.NewRequest(http.MethodGet, "/api/skills/read?id=fixture-skill&path=references%2Fguide.md", nil)
	readRec := httptest.NewRecorder()
	s.handleAPISkillRead(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("handleAPISkillRead expected 200, got %d body=%s", readRec.Code, readRec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(readRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode read payload error: %v", err)
	}
	if payload["path"] != "references/guide.md" {
		t.Fatalf("unexpected read path payload: %+v", payload)
	}
	if payload["content"] != "# Guide\n\nRead me." {
		t.Fatalf("unexpected read content payload: %+v", payload)
	}
}

func TestHandleAPISkillRead_RejectsInvalidPath(t *testing.T) {
	root := t.TempDir()
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	writeSkillResourceFixture(t, filepath.Join(root, "skills"), "fixture-skill")
	s := &Server{skillStore: skillStore}

	req := httptest.NewRequest(http.MethodGet, "/api/skills/read?id=fixture-skill&path=..%2Fsecret.txt", nil)
	rec := httptest.NewRecorder()
	s.handleAPISkillRead(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func writeSkillResourceFixture(t *testing.T, skillsDir, skillID string) {
	t.Helper()
	mustWriteSkillResourceFixture(t, filepath.Join(skillsDir, skillID, "SKILL.md"), "---\nname: \"Fixture\"\ndescription: \"Fixture skill\"\n---\n\nUse fixture skill.\n")
	mustWriteSkillResourceFixture(t, filepath.Join(skillsDir, skillID, "references", "guide.md"), "# Guide\n\nRead me.\n")
	mustWriteSkillResourceFixture(t, filepath.Join(skillsDir, skillID, "scripts", "check.ps1"), "Write-Output 'ok'\n")
}

func mustWriteSkillResourceFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
}
