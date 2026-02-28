package agent

import (
	"strings"
	"testing"
)

func TestNormalizeA2ARequestAndInput_StripsDispatchPhrases(t *testing.T) {
	request, agentInput := normalizeA2ARequestAndInput(
		"codex-local",
		"再次重新调用 codex-local 分析项目 E:\\projects\\ai\\work-notiy，输出技术栈",
		"请调用 codex-local 分析本地项目 E:\\projects\\ai\\work-notiy，输出技术栈与风险",
	)

	for _, got := range []string{request, agentInput} {
		if got == "" {
			t.Fatalf("normalized text should not be empty")
		}
		if strings.Contains(got, "codex-local") ||
			strings.Contains(got, "调用 codex-local") ||
			strings.Contains(got, "再次") ||
			strings.Contains(got, "重新") {
			t.Fatalf("unexpected dispatch phrase remains: %q", got)
		}
	}
}

func TestNormalizeA2ARequestAndInput_FallbackToOriginalWhenFullyStripped(t *testing.T) {
	request, agentInput := normalizeA2ARequestAndInput(
		"codex-local",
		"调用 codex-local",
		"请调用 codex-local",
	)
	if request != "调用 codex-local" {
		t.Fatalf("expected request fallback to original, got %q", request)
	}
	if agentInput != "请调用 codex-local" {
		t.Fatalf("expected agent_input fallback to original, got %q", agentInput)
	}
}
