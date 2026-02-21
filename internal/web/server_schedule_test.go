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
	"laughing-barnacle/internal/scheduler"
	"laughing-barnacle/internal/skills"
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

func TestHandleSettingsScheduleRun(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	if err := store.UpsertScheduledTask(scheduler.Task{
		ID:       "night-reflection-evolution",
		Name:     "night-reflection-evolution",
		Action:   routine.ActionNightReflectionEvolution,
		CronExpr: "0 22 * * *",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertScheduledTask error: %v", err)
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
		ID:       "morning-planning",
		Name:     "morning-planning",
		Action:   routine.ActionMorningPlanning,
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

func TestHandleSettingsScheduleDelete(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	if err := store.UpsertScheduledTask(scheduler.Task{
		ID:       "morning-planning",
		Name:     "morning-planning",
		Action:   routine.ActionMorningPlanning,
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
	form.Set("id", "morning-planning")
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
		if task.ID == "morning-planning" {
			t.Fatalf("expected morning-planning deleted")
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
