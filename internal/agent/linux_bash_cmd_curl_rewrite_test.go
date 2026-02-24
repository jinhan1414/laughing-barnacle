package agent

import (
	"strings"
	"testing"
)

func TestRewriteCmdCurlSettingsPost_DataUrlencode(t *testing.T) {
	raw := `curl -sS -X POST http://127.0.0.1:9080/settings/skills/save --data-urlencode "name=punch-card-reminder" --data-urlencode "prompt=Remind user now"`
	got := rewriteCmdCurlSettingsPost(raw)
	if !strings.Contains(got, `-d "name=punch-card-reminder&prompt=Remind+user+now"`) {
		t.Fatalf("expected rewritten -d payload, got %q", got)
	}
	if !strings.HasSuffix(got, " -D -") {
		t.Fatalf("expected rewritten command to include -D -, got %q", got)
	}
}

func TestRewriteCmdCurlSettingsPost_FormF(t *testing.T) {
	raw := `curl -sS -X POST http://127.0.0.1:9080/settings/skills/save -F "name=punch-card-reminder" -F "prompt=提醒用户打卡"`
	got := rewriteCmdCurlSettingsPost(raw)
	if !strings.Contains(got, "name=punch-card-reminder") || !strings.Contains(got, "prompt=") {
		t.Fatalf("expected rewritten payload to include fields, got %q", got)
	}
}

func TestRewriteCmdCurlSettingsPost_NonTargetUnchanged(t *testing.T) {
	raw := `curl -sS http://127.0.0.1:9080/api/skills`
	if got := rewriteCmdCurlSettingsPost(raw); got != raw {
		t.Fatalf("expected non-target command unchanged, got %q", got)
	}
}

func TestRewriteCmdCurlSettingsPost_AvoidsDoubleEncodingAction(t *testing.T) {
	raw := `curl -sS -X POST http://127.0.0.1:9080/settings/schedules/save -d "id=punch-out&action=skill%3Apunch-card-reminder&cron_expr=32+17+*+*+*"`
	got := rewriteCmdCurlSettingsPost(raw)
	if strings.Contains(got, "skill%253A") {
		t.Fatalf("expected action not to be double-encoded, got %q", got)
	}
	if !strings.Contains(got, "action=skill%3Apunch-card-reminder") {
		t.Fatalf("expected action to stay percent-encoded once, got %q", got)
	}
}

func TestRewriteCmdCurlSettingsPost_SkillSaveUsesIDAsName(t *testing.T) {
	raw := `curl -sS -X POST http://127.0.0.1:9080/settings/skills/save --data-urlencode "id=punch-card-reminder" --data-urlencode "name=打卡提醒" --data-urlencode "prompt=Remind user now"`
	got := rewriteCmdCurlSettingsPost(raw)
	if strings.Contains(got, "id=punch-card-reminder") {
		t.Fatalf("expected id field removed for skills/save, got %q", got)
	}
	if !strings.Contains(got, "name=punch-card-reminder") {
		t.Fatalf("expected name to be normalized from id, got %q", got)
	}
}

func TestRewriteCmdCurlSettingsPost_ScheduleSaveKeepsIDAndEnabled(t *testing.T) {
	raw := `curl -sS -X POST http://127.0.0.1:9080/settings/schedules/save -d "id=clock-out-reminder&name=%E4%B8%8B%E7%8F%AD%E6%89%93%E5%8D%A1&description=%E6%AF%8F%E5%A4%A917%3A32%E6%8F%90%E9%86%92%E6%89%93%E4%B8%8B%E7%8F%AD%E5%8D%A1&action=skill%3Askill&cron_expr=32+17+*+*+*&enabled=on"`
	got := rewriteCmdCurlSettingsPost(raw)
	if !strings.Contains(got, "id=clock-out-reminder") {
		t.Fatalf("expected schedule id to be preserved, got %q", got)
	}
	if !strings.Contains(got, "enabled=on") {
		t.Fatalf("expected enabled field to be preserved, got %q", got)
	}
	if !strings.Contains(got, "action=skill%3Askill") {
		t.Fatalf("expected action encoding to remain valid, got %q", got)
	}
}
