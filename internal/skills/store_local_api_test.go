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
	if !strings.Contains(prompt, "仅接受 id,name,description,action=skill:<skill_id>,cron_expr,enabled=on") {
		t.Fatalf("expected required schedule fields in schedule skill prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "--data-urlencode \"key=value\"") {
		t.Fatalf("expected data-urlencode quoting constraint in schedule skill prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "POST /settings/skills/save（禁止 /api/skills/save）") {
		t.Fatalf("expected skills save endpoint constraint in schedule skill prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "优先使用单条 -d \"k=v&k2=v2\"") {
		t.Fatalf("expected cmd save -d constraint in schedule skill prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "提醒类任务必须先创建或复用专用 reminder skill") {
		t.Fatalf("expected reminder skill binding constraint in schedule skill prompt, got %q", prompt)
	}
}
