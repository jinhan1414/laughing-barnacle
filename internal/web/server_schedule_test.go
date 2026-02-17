package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/routine"
)

type mockScheduleLLM struct {
	responseByPurpose map[string]string
}

func (m *mockScheduleLLM) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	if m.responseByPurpose != nil {
		if out, ok := m.responseByPurpose[req.Purpose]; ok {
			return llm.ChatResponse{Content: out}, nil
		}
	}
	return llm.ChatResponse{Content: `{"reflection":"ok"}`}, nil
}

type mockScheduleRuntime struct {
	ranTaskID string
}

type mockScheduleSkills struct {
	promptByID map[string]string
}

func (m *mockScheduleSkills) ListEnabledSkillIndex() []string {
	return nil
}

func (m *mockScheduleSkills) ReadEnabledSkillPrompt(skillID string) (string, bool) {
	if m == nil || m.promptByID == nil {
		return "", false
	}
	prompt, ok := m.promptByID[strings.TrimSpace(skillID)]
	return prompt, ok
}

func (m *mockScheduleRuntime) Reload() error {
	return nil
}

func (m *mockScheduleRuntime) RunNow(taskID string) error {
	m.ranTaskID = strings.TrimSpace(taskID)
	return nil
}

func TestHandleAPISchedules(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	s := &Server{mcpStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/schedules", nil)
	rec := httptest.NewRecorder()
	s.handleAPISchedules(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Schedules []struct {
			ID       string `json:"id"`
			Action   string `json:"action"`
			CronExpr string `json:"cron_expr"`
		} `json:"schedules"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if len(payload.Schedules) < 2 {
		t.Fatalf("expected default schedules, got %+v", payload.Schedules)
	}
}

func TestHandleSettingsScheduleSave(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	s := &Server{mcpStore: store}
	form := url.Values{}
	form.Set("id", "morning-planning")
	form.Set("name", "晨间规划")
	form.Set("description", "调整到 9 点")
	form.Set("action", routine.ActionMorningPlanning)
	form.Set("cron_expr", "0 9 * * *")
	form.Set("enabled", "on")

	req := httptest.NewRequest(http.MethodPost, "/settings/schedules/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleSettingsScheduleSave(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	if location := rec.Result().Header.Get("Location"); !strings.Contains(location, "section=schedules") {
		t.Fatalf("unexpected redirect location: %q", location)
	}

	tasks := store.ListScheduledTasks()
	found := false
	for _, task := range tasks {
		if task.ID != "morning-planning" {
			continue
		}
		found = true
		if task.CronExpr != "0 9 * * *" {
			t.Fatalf("unexpected cron expr: %q", task.CronExpr)
		}
	}
	if !found {
		t.Fatalf("morning task not found after save")
	}
}

func TestHandleSettingsScheduleRun(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	convStore := conversation.NewStore()
	llmClient := &mockScheduleLLM{
		responseByPurpose: map[string]string{
			"night_reflection_evolution": `{"reflection":"生活：收束。工作：复盘。学习：迭代。"}`,
		},
	}
	agentSvc := agent.New(agent.Config{
		Model:                   "test-model",
		SystemPrompt:            "system",
		CompressionSystemPrompt: "compressor",
		EnforceHumanRoutine:     true,
	}, convStore, llmClient, nil)
	agentSvc.SetHabitProvider(store)
	agentSvc.SetSkillProvider(&mockScheduleSkills{promptByID: map[string]string{
		routine.ScheduledSkillNightReflectionEvolution: "---\nname: \"night\"\ndescription: \"night\"\n---\n\ndo night",
		routine.ScheduledSkillMorningPlanning:          "---\nname: \"morning\"\ndescription: \"morning\"\n---\n\ndo morning",
	}})

	s := &Server{
		mcpStore: store,
		agent:    agentSvc,
	}
	form := url.Values{}
	form.Set("id", "night-reflection-evolution")

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
		if task.ID != "night-reflection-evolution" {
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
		t.Fatalf("expected night task not found")
	}
}

func TestHandleSettingsScheduleRun_UsesSchedulerRuntimeWhenAvailable(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	runtime := &mockScheduleRuntime{}
	s := &Server{
		mcpStore:  store,
		scheduler: runtime,
	}

	form := url.Values{}
	form.Set("id", "morning-planning")
	req := httptest.NewRequest(http.MethodPost, "/settings/schedules/run", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleSettingsScheduleRun(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	if runtime.ranTaskID != "morning-planning" {
		t.Fatalf("expected scheduler runtime called, got %q", runtime.ranTaskID)
	}
}
