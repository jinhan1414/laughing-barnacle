package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"strings"
	"testing"
)

func TestHandleUserMessage_IncludesA2AIndexProgressiveDisclosure(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"chat_reply": {"ok"},
	}}

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
	agentSvc.SetA2AProvider(&mockA2A{
		indexLines: []string{
			"agent_id=codex-local | name=Codex Local | description=本地编码代理 | status=enabled",
		},
	})

	if _, err := agentSvc.HandleUserMessage(context.Background(), "请帮我查下已接入agent"); err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}

	content := ""
	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "A2A 已接入 Agent 索引（渐进式披露）") {
			content = msg.Content
			break
		}
	}
	if strings.TrimSpace(content) == "" {
		t.Fatalf("expected a2a index prompt injected")
	}
	if !strings.Contains(content, "/api/a2a/agents/read?id=<agent_id>") {
		t.Fatalf("expected a2a read hint, got %q", content)
	}
	if !strings.Contains(content, "本索引是本轮固定上下文主来源") {
		t.Fatalf("expected fixed-context hint in a2a index prompt, got %q", content)
	}
	if !strings.Contains(content, "仅在明确需要刷新列表或执行前做一致性校验时") {
		t.Fatalf("expected refresh-on-demand hint in a2a index prompt, got %q", content)
	}
	if strings.Contains(content, "agent_card_json") {
		t.Fatalf("should not inject full agent card content, got %q", content)
	}
}

func TestHandleUserMessage_A2ASendBuiltinToolCall(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "a2a done"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "a2a_send_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      builtinA2ASendToolName,
							Arguments: `{"agent_id":"codex-local","message":"请修复这个 bug"}`,
						},
					},
				},
				nil,
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
		MaxToolCallRounds:          3,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)
	agentSvc.SetA2AProvider(&mockA2A{
		send: A2ATaskResult{
			AgentID: "codex-local",
			TaskID:  "task-123",
			Status:  "submitted",
		},
	})

	reply, err := agentSvc.HandleUserMessage(context.Background(), "调用 codex agent 修复 bug")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "a2a done" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}

	toolResultFound := false
	for _, msg := range fakeLLM.calls[1].Messages {
		if msg.Role == "tool" && strings.Contains(msg.Content, "agent_id: codex-local") && strings.Contains(msg.Content, "task_id: task-123") {
			toolResultFound = true
			break
		}
	}
	if !toolResultFound {
		t.Fatalf("expected a2a tool result in second call")
	}
}
