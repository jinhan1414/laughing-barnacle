package web

import (
	"context"
	"encoding/json"
	"laughing-barnacle/internal/llm"
	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/skills"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
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
	ranTaskID   string
	reloadCalls int
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
	m.reloadCalls++
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
	if len(payload.Schedules) != 0 {
		t.Fatalf("expected no default schedules, got %+v", payload.Schedules)
	}
}

func TestHandleSettingsScheduleSave(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	s := &Server{mcpStore: store}
	form := url.Values{}
	form.Set("id", "daily-review")
	form.Set("name", "日终复盘")
	form.Set("description", "调整到 9 点")
	form.Set("action", "skill:daily-review")
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
		if task.ID != "daily-review" {
			continue
		}
		found = true
		if task.CronExpr != "0 9 * * *" {
			t.Fatalf("unexpected cron expr: %q", task.CronExpr)
		}
	}
	if !found {
		t.Fatalf("daily-review task not found after save")
	}
}

func TestHandleSettingsScheduleSave_RejectsUnknownSkillAction(t *testing.T) {
	root := t.TempDir()
	store, err := mcp.NewStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("New skill store error: %v", err)
	}

	s := &Server{mcpStore: store, skillStore: skillStore}
	form := url.Values{}
	form.Set("id", "greeting-task")
	form.Set("name", "问候任务")
	form.Set("description", "测试不存在 skill")
	form.Set("action", "skill:greeting")
	form.Set("cron_expr", "*/5 * * * *")
	form.Set("enabled", "on")

	req := httptest.NewRequest(http.MethodPost, "/settings/schedules/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleSettingsScheduleSave(rec, req)

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
	for _, task := range tasks {
		if task.ID == "greeting-task" {
			t.Fatalf("unexpected task persisted for unknown skill action: %+v", task)
		}
	}
}
