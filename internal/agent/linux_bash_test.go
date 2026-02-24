package agent

import "testing"

func TestNormalizeCommandForShell_CmdUnescapesQuotes(t *testing.T) {
	raw := `curl -s \"http://127.0.0.1:9080/api/schedules\"`
	got := normalizeCommandForShell("cmd", raw)
	want := `curl -s "http://127.0.0.1:9080/api/schedules"`
	if got != want {
		t.Fatalf("normalizeCommandForShell mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestShouldHintCmdCurlQuoteFix(t *testing.T) {
	if !shouldHintCmdCurlQuoteFix("cmd", 3, `curl -s \"http://127.0.0.1:9080/api/schedules\"`, "") {
		t.Fatalf("expected quote-fix hint for cmd curl exit=3 with empty stderr")
	}
	if shouldHintCmdCurlQuoteFix("cmd", 0, "curl -s http://127.0.0.1:9080/api/schedules", "") {
		t.Fatalf("unexpected hint for success case")
	}
}

func TestShouldHintScheduleSaveReadback(t *testing.T) {
	if !shouldHintScheduleSaveReadback("cmd", 0, "curl -sS -X POST http://127.0.0.1:9080/settings/schedules/save -d \"a=b\"", "") {
		t.Fatalf("expected readback hint for cmd schedule save with empty stdout")
	}
	if shouldHintScheduleSaveReadback("cmd", 0, "curl -sS http://127.0.0.1:9080/api/schedules", "") {
		t.Fatalf("unexpected readback hint for non-save endpoint")
	}
}

func TestNormalizeCommandForShell_CmdAppendsHeadersForSettingsCurl(t *testing.T) {
	raw := `curl -sS -X POST http://127.0.0.1:9080/settings/schedules/save --data-urlencode "id=punch-in"`
	got := normalizeCommandForShell("cmd", raw)
	if got != raw+" -D -" {
		t.Fatalf("expected -D - to be appended for settings curl, got %q", got)
	}
}

func TestExtractRedirectErrorFromCurlHeaders(t *testing.T) {
	headers := "HTTP/1.1 302 Found\r\nLocation: /settings?section=schedules&error=task+id+is+required\r\n\r\n"
	got := extractRedirectErrorFromCurlHeaders(headers)
	want := "settings 接口返回错误：task id is required"
	if got != want {
		t.Fatalf("extractRedirectErrorFromCurlHeaders mismatch: got %q, want %q", got, want)
	}
}
