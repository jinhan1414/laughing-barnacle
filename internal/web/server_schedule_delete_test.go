package web

import (
	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/scheduler"
	"laughing-barnacle/internal/skills"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleSettingsScheduleDelete(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	if err := store.UpsertScheduledTask(scheduler.Task{
		ID:       "daily-review",
		Name:     "daily-review",
		Action:   "skill:daily-review",
		CronExpr: "30 8 * * *",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertScheduledTask error: %v", err)
	}
	runtime := &mockScheduleRuntime{}
	s := &Server{
		mcpStore:  store,
		scheduler: runtime,
	}

	form := url.Values{}
	form.Set("id", "daily-review")
	req := httptest.NewRequest(http.MethodPost, "/settings/schedules/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleSettingsScheduleDelete(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	if location := rec.Result().Header.Get("Location"); !strings.Contains(location, "section=schedules") {
		t.Fatalf("unexpected redirect location: %q", location)
	}
	if runtime.reloadCalls != 1 {
		t.Fatalf("expected scheduler reload once, got %d", runtime.reloadCalls)
	}
	for _, task := range store.ListScheduledTasks() {
		if task.ID == "daily-review" {
			t.Fatalf("expected daily-review deleted")
		}
	}
}

func TestHandleSettingsScheduleDelete_DeletesCorrespondingSkill(t *testing.T) {
	root := t.TempDir()
	store, err := mcp.NewStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("New skill store error: %v", err)
	}
	if err := skillStore.UpsertSkill(skills.Skill{
		ID:          "cleanup-skill",
		Name:        "cleanup-skill",
		Description: "cleanup skill",
		Prompt:      "do cleanup",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("UpsertSkill error: %v", err)
	}
	if err := store.UpsertScheduledTask(scheduler.Task{
		ID:       "cleanup-task",
		Name:     "cleanup-task",
		Action:   "skill:cleanup-skill",
		CronExpr: "*/10 * * * *",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertScheduledTask error: %v", err)
	}

	s := &Server{
		mcpStore:   store,
		skillStore: skillStore,
	}

	form := url.Values{}
	form.Set("id", "cleanup-task")
	req := httptest.NewRequest(http.MethodPost, "/settings/schedules/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleSettingsScheduleDelete(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	for _, task := range store.ListScheduledTasks() {
		if task.ID == "cleanup-task" {
			t.Fatalf("expected cleanup-task deleted")
		}
	}
	for _, skill := range skillStore.ListSkills() {
		if skill.ID == "cleanup-skill" {
			t.Fatalf("expected cleanup-skill deleted")
		}
	}
}

func TestHandleSettingsScheduleDelete_KeepSharedSkill(t *testing.T) {
	root := t.TempDir()
	store, err := mcp.NewStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("New skill store error: %v", err)
	}
	if err := skillStore.UpsertSkill(skills.Skill{
		ID:          "shared-skill",
		Name:        "shared-skill",
		Description: "shared skill",
		Prompt:      "do shared",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("UpsertSkill error: %v", err)
	}
	if err := store.UpsertScheduledTask(scheduler.Task{
		ID:       "shared-task-a",
		Name:     "shared-task-a",
		Action:   "skill:shared-skill",
		CronExpr: "*/5 * * * *",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertScheduledTask task-a error: %v", err)
	}
	if err := store.UpsertScheduledTask(scheduler.Task{
		ID:       "shared-task-b",
		Name:     "shared-task-b",
		Action:   "skill:shared-skill",
		CronExpr: "*/7 * * * *",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertScheduledTask task-b error: %v", err)
	}

	s := &Server{
		mcpStore:   store,
		skillStore: skillStore,
	}
	form := url.Values{}
	form.Set("id", "shared-task-a")
	req := httptest.NewRequest(http.MethodPost, "/settings/schedules/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleSettingsScheduleDelete(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	foundTaskB := false
	for _, task := range store.ListScheduledTasks() {
		if task.ID == "shared-task-a" {
			t.Fatalf("expected shared-task-a deleted")
		}
		if task.ID == "shared-task-b" {
			foundTaskB = true
		}
	}
	if !foundTaskB {
		t.Fatalf("expected shared-task-b still exists")
	}

	foundSharedSkill := false
	for _, skill := range skillStore.ListSkills() {
		if skill.ID == "shared-skill" {
			foundSharedSkill = true
			break
		}
	}
	if !foundSharedSkill {
		t.Fatalf("expected shared-skill kept because still referenced")
	}
}

func TestHandleSettingsScheduleDelete_KeepBuiltinSkill(t *testing.T) {
	root := t.TempDir()
	store, err := mcp.NewStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("New skill store error: %v", err)
	}
	if err := store.UpsertScheduledTask(scheduler.Task{
		ID:       "mcp-config-task",
		Name:     "mcp-config-task",
		Action:   "skill:mcp-config-maintainer",
		CronExpr: "30 8 * * *",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertScheduledTask error: %v", err)
	}

	s := &Server{
		mcpStore:   store,
		skillStore: skillStore,
	}
	form := url.Values{}
	form.Set("id", "mcp-config-task")
	req := httptest.NewRequest(http.MethodPost, "/settings/schedules/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleSettingsScheduleDelete(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	found := false
	for _, skill := range skillStore.ListSkills() {
		if skill.ID == "mcp-config-maintainer" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected builtin mcp-config-maintainer skill to be kept")
	}
}
