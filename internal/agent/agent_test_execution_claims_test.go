package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"strings"
	"testing"
)

func TestHandleUserMessage_UsesStandardToolExecutionForRuntimeScheduleQuery(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"先查询任务列表。", "当前有 0 个定时任务。"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "call_sched_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      builtinLinuxBashToolName,
							Arguments: `"curl -s http://127.0.0.1:8080/api/schedules"`,
						},
					},
				},
				nil,
			},
		},
	}
	prevRunLinuxBashFn := runLinuxBashFn
	runLinuxBashFn = func(_ context.Context, req linuxBashRequest) (string, error) {
		if strings.TrimSpace(req.Command) != "curl -s http://127.0.0.1:8080/api/schedules" {
			t.Fatalf("unexpected command: %q", req.Command)
		}
		return "exit_code: 0\nshell: bash\nstdout:\n{\"schedules\":[]}\nstderr:\n(无)", nil
	}
	t.Cleanup(func() { runLinuxBashFn = prevRunLinuxBashFn })

	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          2,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "有哪些定时任务")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if strings.TrimSpace(reply) == "" {
		t.Fatalf("expected non-empty reply")
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected two llm calls (tool + final reply), got %d", len(fakeLLM.calls))
	}

	_, messages := store.Snapshot()
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("unexpected messages: %+v", messages)
	}
	if len(messages[0].ToolCalls) != 1 || messages[0].ToolCalls[0].Name != builtinLinuxBashToolName {
		t.Fatalf("expected one persisted tool call, got %+v", messages[0].ToolCalls)
	}
}
