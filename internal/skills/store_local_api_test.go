package skills

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSetLocalAPIBaseURL_RewritesBuiltinSkillPrompt(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.SetLocalAPIBaseURL(":9080"); err != nil {
		t.Fatalf("SetLocalAPIBaseURL error: %v", err)
	}

	prompt, ok := store.ReadEnabledSkillPrompt("schedule-config-maintainer")
	if !ok {
		t.Fatalf("expected builtin skill prompt to be readable")
	}
	if !strings.Contains(prompt, "http://127.0.0.1:9080/api/skills") {
		t.Fatalf("expected prompt to use :9080 local api base, got %q", prompt)
	}
	if strings.Contains(prompt, legacyLocalAPIBaseURL) {
		t.Fatalf("expected legacy base url to be replaced, got %q", prompt)
	}
	if !strings.Contains(prompt, "工具参数键必须是 command（不是 cmd）") {
		t.Fatalf("expected command key constraint in schedule skill prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "禁止 /api/schedules/list") {
		t.Fatalf("expected schedules endpoint constraint in schedule skill prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "禁止反斜杠转义引号") {
		t.Fatalf("expected windows quote escape constraint in schedule skill prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "仅接受 id,name,description,action=skill:<skill_id>,cron_expr,enabled") {
		t.Fatalf("expected required schedule fields in schedule skill prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "Content-Type: application/json") {
		t.Fatalf("expected json content-type constraint in schedule skill prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "POST /api/skills/save（JSON）") {
		t.Fatalf("expected skills save json endpoint constraint in schedule skill prompt, got %q", prompt)
	}
	if strings.Contains(prompt, "--data-urlencode") {
		t.Fatalf("expected schedule skill prompt to remove data-urlencode default, got %q", prompt)
	}
	if !strings.Contains(prompt, "提醒类任务必须先创建或复用专用 reminder skill") {
		t.Fatalf("expected reminder skill binding constraint in schedule skill prompt, got %q", prompt)
	}
}

func TestBuiltinMaintenanceSkills_UseJSONProtocol(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	cases := []struct {
		id       string
		contains string
	}{
		{id: "mcp-config-maintainer", contains: "/api/mcp/services/save"},
		{id: "skills-config-maintainer", contains: "/api/skills/save"},
		{id: "schedule-config-maintainer", contains: "/api/schedules/save"},
	}

	for _, tc := range cases {
		prompt, ok := store.ReadEnabledSkillPrompt(tc.id)
		if !ok {
			t.Fatalf("expected builtin skill %s prompt readable", tc.id)
		}
		if !strings.Contains(prompt, tc.contains) {
			t.Fatalf("expected prompt %s to contain %q, got %q", tc.id, tc.contains, prompt)
		}
		if strings.Contains(prompt, "--data-urlencode") {
			t.Fatalf("expected prompt %s to avoid data-urlencode default, got %q", tc.id, prompt)
		}
	}
}

func TestBuiltinA2ATaskOrchestrator_UsesInjectedIndexFirst(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.SetLocalAPIBaseURL(":9080"); err != nil {
		t.Fatalf("SetLocalAPIBaseURL error: %v", err)
	}

	prompt, ok := store.ReadEnabledSkillPrompt("a2a-task-orchestrator")
	if !ok {
		t.Fatalf("expected builtin skill prompt to be readable")
	}
	if !strings.Contains(prompt, "系统已注入的“已启用 A2A Agent 索引”") {
		t.Fatalf("expected injected-index first hint, got %q", prompt)
	}
	if !strings.Contains(prompt, "/api/a2a/agents/read?id=<agent_id>") {
		t.Fatalf("expected detail-read hint, got %q", prompt)
	}
	if !strings.Contains(prompt, "仅在用户明确要求刷新列表或执行前需要一致性校验时") {
		t.Fatalf("expected on-demand list refresh constraint, got %q", prompt)
	}
	if strings.Contains(prompt, "步骤 1：先读索引：curl -sS http://127.0.0.1:9080/api/a2a/agents") {
		t.Fatalf("expected no mandatory list-read first step, got %q", prompt)
	}
}
