package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"strings"
	"testing"
)

func TestHandleUserMessage_WithToolCalls(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "weather ready"},
		},
		usages: map[string][]llm.TokenUsage{
			"chat_reply": {
				{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
				{PromptTokens: 80, CompletionTokens: 30, TotalTokens: 110},
			},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "call_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      "weather__query",
							Arguments: `{"city":"beijing"}`,
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
			`weather__query:{"city":"beijing"}`: `{"temp":18}`,
		},
	}

	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          4,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, fakeTools)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "今天北京天气")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "weather ready" {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}
	if len(fakeLLM.calls[0].Tools) < 2 {
		t.Fatalf("expected builtin bash + external tools, got %d", len(fakeLLM.calls[0].Tools))
	}
	foundBash := false
	foundWeather := false
	for _, tool := range fakeLLM.calls[0].Tools {
		if tool.Function.Name == builtinLinuxBashToolName {
			foundBash = true
		}
		if tool.Function.Name == "weather__query" {
			foundWeather = true
		}
	}
	if !foundBash || !foundWeather {
		t.Fatalf("expected both bash and weather__query tools, got %+v", fakeLLM.calls[0].Tools)
	}
	if len(fakeTools.calls) != 1 || fakeTools.calls[0].Function.Name != "weather__query" {
		t.Fatalf("unexpected tool calls: %+v", fakeTools.calls)
	}
	_, messages := store.Snapshot()
	if len(messages) != 2 {
		t.Fatalf("expected user + assistant messages, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 1 {
		t.Fatalf("expected tool calls attached to user message, got %d", len(messages[0].ToolCalls))
	}
	if messages[0].ToolCalls[0].Name != "weather__query" {
		t.Fatalf("unexpected attached tool name: %s", messages[0].ToolCalls[0].Name)
	}
	if messages[1].Usage == nil {
		t.Fatalf("expected assistant usage in messages")
	}
	if messages[1].Usage.PromptTokens != 180 || messages[1].Usage.CompletionTokens != 50 || messages[1].Usage.TotalTokens != 230 {
		t.Fatalf("unexpected aggregated usage: %+v", messages[1].Usage)
	}
}

func TestHandleUserMessage_AllowsFinalReplyAfterMaxToolCallRounds(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "", "weather final"},
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
			`weather__query:{"city":"beijing","day":2}`: `{"temp":19}`,
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
	}, store, fakeLLM, fakeTools)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "两天北京天气")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "weather final" {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(fakeLLM.calls) != 3 {
		t.Fatalf("expected 3 llm calls (2 tool rounds + final reply), got %d", len(fakeLLM.calls))
	}
	if len(fakeTools.calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(fakeTools.calls))
	}

	_, messages := store.Snapshot()
	if len(messages) != 2 {
		t.Fatalf("expected user + assistant messages, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 2 {
		t.Fatalf("expected 2 recorded tool calls, got %d", len(messages[0].ToolCalls))
	}
}

func TestHandleUserMessage_AddsNearToolBudgetPromptBeforeFinalRound(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "weather ready"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "call_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      "weather__query",
							Arguments: `{"city":"beijing"}`,
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
				Type:     "function",
				Function: llm.ToolFunctionDefinition{Name: "weather__query"},
			},
		},
		response: map[string]string{
			`weather__query:{"city":"beijing"}`: `{"temp":18}`,
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
	}, store, fakeLLM, fakeTools)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "查天气")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "weather ready" {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}

	found := false
	for _, msg := range fakeLLM.calls[1].Messages {
		if msg.Role != "system" {
			continue
		}
		if strings.Contains(msg.Content, "工具调用预算即将耗尽") &&
			strings.Contains(msg.Content, "autonomous_run__checkpoint") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected near-budget autonomous-run handoff prompt in second llm call")
	}
}
