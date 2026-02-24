package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"strings"
	"testing"
)

func TestHandleUserMessage_ForcesFinalAnswerWhenToolRoundsExceeded(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "", "forced final"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "call_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      "weather__query",
							Arguments: `{"city":"beijing","day":1}`,
						},
					},
				},
				{
					{
						ID:   "call_2",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      "weather__query",
							Arguments: `{"city":"beijing","day":2}`,
						},
					},
				},
				nil,
			},
		},
	}
	fakeTools := &mockTools{
		listed: []llm.ToolDefinition{
			{
				Type: "function",
				Function: llm.ToolFunctionDefinition{
					Name: "weather__query",
				},
			},
		},
		response: map[string]string{
			`weather__query:{"city":"beijing","day":1}`: `{"temp":18}`,
		},
	}

	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          1,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, fakeTools)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "两天北京天气")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "forced final" {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(fakeTools.calls) != 1 {
		t.Fatalf("expected only one tool execution before hitting cap, got %d", len(fakeTools.calls))
	}
	if len(fakeLLM.calls) != 3 {
		t.Fatalf("expected 3 llm calls (1 tool round + capped fallback), got %d", len(fakeLLM.calls))
	}
	if len(fakeLLM.calls[2].Tools) != 0 {
		t.Fatalf("expected capped fallback call without tools, got %d tools", len(fakeLLM.calls[2].Tools))
	}
}

func TestHandleUserMessage_DoesNotReplayToolCallsFromPreviousCompletedTurn(t *testing.T) {
	store := conversation.NewStore()
	store.Append("user", "今天北京天气")
	if err := store.SetLatestUserToolCalls([]conversation.ToolCall{
		{
			ID:        "call_prev_1",
			Name:      "weather__query",
			Arguments: `{"city":"beijing"}`,
			Result:    `{"temp":18}`,
		},
	}); err != nil {
		t.Fatalf("SetLatestUserToolCalls error: %v", err)
	}

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

	reply, err := agentSvc.HandleUserMessage(context.Background(), "继续总结下这个天气")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}

	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) == 1 {
			t.Fatalf("expected no replayed assistant tool_call message, got %+v", msg.ToolCalls)
		}
		if msg.Role == "tool" &&
			msg.ToolCallID == "call_prev_1" {
			t.Fatalf("expected no replayed tool result message, got %+v", msg)
		}
		if msg.Role == "system" && strings.Contains(msg.Content, "可复用工具结果（最近）") {
			t.Fatalf("unexpected carryover system hint: %q", msg.Content)
		}
	}
}

func TestRetryLastUserMessage_ReplaysToolCallsFromPendingUserMessage(t *testing.T) {
	store := conversation.NewStore()
	store.Append("user", "继续总结天气")
	if err := store.SetLatestUserToolCalls([]conversation.ToolCall{
		{
			ID:        "call_prev_1",
			Name:      "weather__query",
			Arguments: `{"city":"beijing"}`,
			Result:    `{"temp":18}`,
		},
	}); err != nil {
		t.Fatalf("SetLatestUserToolCalls error: %v", err)
	}

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

	reply, err := agentSvc.RetryLastUserMessage(context.Background())
	if err != nil {
		t.Fatalf("RetryLastUserMessage error: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}

	foundHistoryAssistantToolCall := false
	foundHistoryToolResult := false
	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) == 1 {
			call := msg.ToolCalls[0]
			if call.ID == "call_prev_1" &&
				call.Function.Name == "weather__query" &&
				call.Function.Arguments == `{"city":"beijing"}` {
				foundHistoryAssistantToolCall = true
			}
		}
		if msg.Role == "tool" &&
			msg.ToolCallID == "call_prev_1" &&
			msg.Content == `{"temp":18}` {
			foundHistoryToolResult = true
		}
	}
	if !foundHistoryAssistantToolCall {
		t.Fatalf("expected replayed assistant tool_call message on retry")
	}
	if !foundHistoryToolResult {
		t.Fatalf("expected replayed tool result message on retry")
	}
}
