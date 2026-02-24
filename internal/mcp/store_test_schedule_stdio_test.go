package mcp

import (
	"laughing-barnacle/internal/routine"
	"laughing-barnacle/internal/scheduler"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreDefaultScheduledTasks_EmptyByDefault(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	tasks := store.ListScheduledTasks()
	if len(tasks) != 0 {
		t.Fatalf("expected no default scheduled tasks, got %+v", tasks)
	}

	reloaded, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}
	if len(reloaded.ListScheduledTasks()) != 0 {
		t.Fatalf("expected no default scheduled tasks after reload")
	}
}

func TestStoreLoad_LegacySettingsWithoutSchedules_KeepEmpty(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	raw := `{
  "mcp": {
    "services": []
  },
  "skills": {
    "items": []
  },
  "agent": {
    "prompts": {},
    "habits": {}
  }
}`
	if err := os.WriteFile(settingsPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("write settings file error: %v", err)
	}

	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore legacy settings error: %v", err)
	}

	tasks := store.ListScheduledTasks()
	if len(tasks) != 0 {
		t.Fatalf("expected empty schedules for legacy settings, got %+v", tasks)
	}
}

func TestStoreUpsertScheduledTaskAndMarkRun(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertScheduledTask(scheduler.Task{
		ID:          "morning-planning",
		Name:        "晨间规划",
		Description: "更新后的晨间任务",
		Action:      routine.ActionMorningPlanning,
		CronExpr:    "0 9 * * *",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("UpsertScheduledTask error: %v", err)
	}

	runAt := time.Date(2026, 2, 17, 9, 0, 0, 0, time.Local)
	if err := store.MarkScheduledTaskRun("morning-planning", runAt, "success", ""); err != nil {
		t.Fatalf("MarkScheduledTaskRun error: %v", err)
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
		if task.LastRunAt.IsZero() || !task.LastRunAt.Equal(runAt) {
			t.Fatalf("unexpected last run at: %v", task.LastRunAt)
		}
		if task.LastStatus != "success" {
			t.Fatalf("unexpected last status: %q", task.LastStatus)
		}
	}
	if !found {
		t.Fatalf("expected updated morning task")
	}
}

func TestStoreDeleteScheduledTask(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.DeleteScheduledTask("morning-planning"); err != nil {
		t.Fatalf("DeleteScheduledTask error: %v", err)
	}

	for _, task := range store.ListScheduledTasks() {
		if task.ID == "morning-planning" {
			t.Fatalf("expected morning-planning deleted")
		}
	}
}

func TestStoreUpsertScheduledTask_InvalidCronRejected(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	err = store.UpsertScheduledTask(scheduler.Task{
		Name:     "bad task",
		Action:   routine.ActionMorningPlanning,
		CronExpr: "invalid-cron",
		Enabled:  true,
	})
	if err == nil {
		t.Fatalf("expected invalid cron to be rejected")
	}
}

func TestStoreUpsertScheduledTask_LegacyActionRejected(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	err = store.UpsertScheduledTask(scheduler.Task{
		ID:       "legacy-action-task",
		Name:     "legacy action",
		Action:   "morning_planning",
		CronExpr: "5 9 * * *",
		Enabled:  true,
	})
	if err == nil {
		t.Fatalf("expected legacy underscore action rejected")
	}
}

func TestStoreUpsertScheduledTask_SkillActionWithoutID_DoesNotOverwrite(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	err = store.UpsertScheduledTask(scheduler.Task{
		Name:     "上班打卡",
		Action:   "skill:skill",
		CronExpr: "56 8 * * *",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("first UpsertScheduledTask error: %v", err)
	}
	err = store.UpsertScheduledTask(scheduler.Task{
		Name:     "下班打卡",
		Action:   "skill:skill",
		CronExpr: "32 17 * * *",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("second UpsertScheduledTask error: %v", err)
	}

	tasks := store.ListScheduledTasks()
	if len(tasks) != 2 {
		t.Fatalf("expected two tasks, got %+v", tasks)
	}
	if tasks[0].ID == tasks[1].ID {
		t.Fatalf("expected different generated IDs, got %+v", tasks)
	}
}

func TestStoreUpsertService_StdioPersisted(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertService(Service{
		Name:      "Filesystem MCP",
		Transport: "stdio",
		Command:   "npx",
		Args:      []string{"-y", "@modelcontextprotocol/server-filesystem", "/workspace"},
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService stdio error: %v", err)
	}

	reloaded, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}

	services := reloaded.ListServices()
	if len(services) != 1 {
		t.Fatalf("expected one service, got %d", len(services))
	}
	if services[0].Transport != ServiceTransportStdio {
		t.Fatalf("unexpected transport: %q", services[0].Transport)
	}
	if services[0].Command != "npx" {
		t.Fatalf("unexpected command: %q", services[0].Command)
	}
	if len(services[0].Args) != 3 {
		t.Fatalf("unexpected args: %+v", services[0].Args)
	}
}

func TestStoreUpsertService_StdioRequiresCommand(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	err = store.UpsertService(Service{
		Name:      "Broken stdio",
		Transport: "stdio",
		Enabled:   true,
	})
	if err == nil {
		t.Fatalf("expected stdio command required error")
	}
	if !strings.Contains(err.Error(), "service command is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
