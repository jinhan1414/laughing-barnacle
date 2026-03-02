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
	if !strings.Contains(prompt, "context__read(resource=\"schedules\", action=\"list\")") {
		t.Fatalf("expected schedules list to route via context__read in schedule skill prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "仅接受 id,name,description,action=skill:<skill_id>,cron_expr,enabled") {
		t.Fatalf("expected required schedule fields in schedule skill prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "context__read") || !strings.Contains(prompt, "maintenance__write") {
		t.Fatalf("expected schedule skill prompt to use native tools, got %q", prompt)
	}
	if strings.Contains(prompt, "curl") || strings.Contains(prompt, "Invoke-RestMethod") {
		t.Fatalf("expected schedule skill prompt to remove shell api instructions, got %q", prompt)
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
		{id: "mcp-config-maintainer", contains: "maintenance__write(resource=\"mcp\""},
		{id: "skills-config-maintainer", contains: "maintenance__write(resource=\"skills\""},
		{id: "schedule-config-maintainer", contains: "maintenance__write(resource=\"schedules\""},
	}

	for _, tc := range cases {
		prompt, ok := store.ReadEnabledSkillPrompt(tc.id)
		if !ok {
			t.Fatalf("expected builtin skill %s prompt readable", tc.id)
		}
		if !strings.Contains(prompt, tc.contains) {
			t.Fatalf("expected prompt %s to contain %q, got %q", tc.id, tc.contains, prompt)
		}
		if !strings.Contains(prompt, "context__read") {
			t.Fatalf("expected prompt %s to include context__read, got %q", tc.id, prompt)
		}
		if strings.Contains(prompt, "curl") || strings.Contains(prompt, "Invoke-RestMethod") {
			t.Fatalf("expected prompt %s to avoid shell api calls, got %q", tc.id, prompt)
		}
	}
}

func TestBuiltinMCPConfigMaintainer_UsesTransportScopedConfig(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	prompt, ok := store.ReadEnabledSkillPrompt("mcp-config-maintainer")
	if !ok {
		t.Fatalf("expected builtin skill prompt readable")
	}
	if !strings.Contains(prompt, "transport:\"streamable_http|sse\"") {
		t.Fatalf("expected http transport save hint, got %q", prompt)
	}
	if !strings.Contains(prompt, "headers,enabled") {
		t.Fatalf("expected headers field hint, got %q", prompt)
	}
	if !strings.Contains(prompt, "transport:\"stdio\"") || !strings.Contains(prompt, "env,enabled") {
		t.Fatalf("expected stdio env save hint, got %q", prompt)
	}
	if !strings.Contains(prompt, "headers 禁止写 Authorization") {
		t.Fatalf("expected authorization header constraint, got %q", prompt)
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
	if !strings.Contains(prompt, "context__read(resource=\"a2a\", action=\"read\", id=\"<agent_id>\")") {
		t.Fatalf("expected detail-read hint, got %q", prompt)
	}
	if !strings.Contains(prompt, "仅在用户明确要求刷新列表或执行前需要一致性校验时") {
		t.Fatalf("expected on-demand list refresh constraint, got %q", prompt)
	}
	if !strings.Contains(prompt, "agent_input 直接描述目标任务与验收标准") {
		t.Fatalf("expected direct agent_input objective constraint, got %q", prompt)
	}
	if !strings.Contains(prompt, "禁止出现“调用 <agent_id>”") {
		t.Fatalf("expected no-dispatch wording for agent_input, got %q", prompt)
	}
	if strings.Contains(prompt, "curl -sS") {
		t.Fatalf("expected no mandatory list-read first step, got %q", prompt)
	}
}
