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

func TestNeedsExecutionClaimCorrection(t *testing.T) {
	writeCall := conversation.ToolCall{
		Name:      builtinLinuxBashToolName,
		Arguments: `"curl -s -X POST http://127.0.0.1:8080/api/skills/save -H 'Content-Type: application/json' -d '{}'"`,
		Result:    "exit_code: 0\nshell: bash\nstdout:\nok\nstderr:\n(无)",
	}
	readbackCall := conversation.ToolCall{
		Name:      builtinLinuxBashToolName,
		Arguments: `"curl -s http://127.0.0.1:8080/api/skills"`,
		Result:    "exit_code: 0\nshell: bash\nstdout:\n{\"skills\":[]}\nstderr:\n(无)",
	}

	tests := []struct {
		name     string
		reply    string
		calls    []conversation.ToolCall
		expected bool
	}{
		{
			name:     "success claim without tool evidence",
			reply:    "已成功创建 reminder Skill。",
			calls:    nil,
			expected: true,
		},
		{
			name:     "success claim with write but no readback",
			reply:    "已成功创建 reminder Skill。",
			calls:    []conversation.ToolCall{writeCall},
			expected: true,
		},
		{
			name:  "success claim with write and readback",
			reply: "已成功创建 reminder Skill，并已校验可见。",
			calls: []conversation.ToolCall{
				writeCall,
				readbackCall,
			},
			expected: false,
		},
		{
			name:  "contradictory success and failure",
			reply: "已成功创建 reminder Skill，但列表未出现该条目。",
			calls: []conversation.ToolCall{
				writeCall,
				readbackCall,
			},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := needsExecutionClaimCorrection(tc.reply, tc.calls)
			if got != tc.expected {
				t.Fatalf("unexpected correction decision: got=%v expected=%v", got, tc.expected)
			}
		})
	}
}

func TestHandleUserMessage_RewritesUnverifiedSuccessClaim(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {
				"已成功创建 reminder Skill。",
				"我还没完成创建；需要先执行写入并回读校验。",
			},
		},
	}

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

	reply, err := agentSvc.HandleUserMessage(context.Background(), "每天8点55提醒我打上班卡")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if strings.Contains(reply, "已成功创建") {
		t.Fatalf("reply should be corrected, got %q", reply)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls (reply + rewrite), got %d", len(fakeLLM.calls))
	}
	for _, call := range fakeLLM.calls {
		if call.Purpose == "chat_action_plan" {
			t.Fatalf("should not call two-phase action planning in normal flow")
		}
	}
}
