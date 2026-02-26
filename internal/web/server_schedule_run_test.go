package web

import (
	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
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

func TestHandleSettingsScheduleRun(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	if err := store.UpsertScheduledTask(scheduler.Task{
		ID:       "daily-review",
		Name:     "daily-review",
		Action:   "skill:daily-review",
		CronExpr: "0 22 * * *",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertScheduledTask error: %v", err)
	}

	convStore := conversation.NewStore()
	llmClient := &mockScheduleLLM{
		responseByPurpose: map[string]string{
			"scheduled_skill_daily_review": `{"content":"今日总结：生活稳定，工作推进，学习迭代。"}`,
		},
	}
	agentSvc := agent.New(agent.Config{
		Model:                   "test-model",
		SystemPrompt:            "system",
		CompressionSystemPrompt: "compressor",
		EnforceHumanRoutine:     true,
	}, convStore, llmClient, nil)
	agentSvc.SetSkillProvider(&mockScheduleSkills{promptByID: map[string]string{
		"daily-review": "---\nname: \"daily\"\ndescription: \"daily\"\n---\n\ndo daily review",
	}})

	s := &Server{
		mcpStore: store,
		agent:    agentSvc,
	}
	form := url.Values{}
	form.Set("id", "daily-review")

	req := httptest.NewRequest(http.MethodPost, "/settings/schedules/run", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleSettingsScheduleRun(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	if location := rec.Result().Header.Get("Location"); !strings.Contains(location, "section=schedules") {
		t.Fatalf("unexpected redirect location: %q", location)
	}
	if location := rec.Result().Header.Get("Location"); !strings.Contains(location, "success=") {
		t.Fatalf("expected success hint in redirect location: %q", location)
	}

	tasks := store.ListScheduledTasks()
	found := false
	for _, task := range tasks {
		if task.ID != "daily-review" {
			continue
		}
		found = true
		if task.LastStatus != "success" {
			t.Fatalf("expected success status, got %q", task.LastStatus)
		}
		if task.LastRunAt.IsZero() {
			t.Fatalf("expected non-zero last run time")
		}
		if task.LastMessage != "manual_run" {
			t.Fatalf("expected manual run marker, got %q", task.LastMessage)
		}
	}
	if !found {
		t.Fatalf("expected daily-review task not found")
	}
}

func TestHandleSettingsScheduleRun_RejectsUnknownSkillAction(t *testing.T) {
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
		ID:       "greeting-task",
		Name:     "greeting-task",
		Action:   "skill:greeting",
		CronExpr: "*/5 * * * *",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertScheduledTask error: %v", err)
	}

	s := &Server{
		mcpStore:   store,
		skillStore: skillStore,
	}
	form := url.Values{}
	form.Set("id", "greeting-task")

	req := httptest.NewRequest(http.MethodPost, "/settings/schedules/run", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleSettingsScheduleRun(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	location := rec.Result().Header.Get("Location")
	if !strings.Contains(location, "section=schedules") {
		t.Fatalf("unexpected redirect location: %q", location)
	}
	if !strings.Contains(location, "error=") {
		t.Fatalf("expected error hint in redirect location: %q", location)
	}

	tasks := store.ListScheduledTasks()
	found := false
	for _, task := range tasks {
		if task.ID != "greeting-task" {
			continue
		}
		found = true
		if task.LastStatus != "error" {
			t.Fatalf("expected error status, got %q", task.LastStatus)
		}
		if strings.TrimSpace(task.LastMessage) == "" {
			t.Fatalf("expected non-empty error message")
		}
	}
	if !found {
		t.Fatalf("expected greeting task not found")
	}
}

func TestHandleSettingsScheduleRun_UsesSchedulerRuntimeWhenAvailable(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, "/settings/schedules/run", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleSettingsScheduleRun(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	if runtime.ranTaskID != "daily-review" {
		t.Fatalf("expected scheduler runtime called, got %q", runtime.ranTaskID)
	}
}
