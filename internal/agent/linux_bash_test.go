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
