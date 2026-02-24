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
