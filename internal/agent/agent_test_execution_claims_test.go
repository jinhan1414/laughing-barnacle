package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleUserMessage_UsesStandardToolExecutionForRuntimeScheduleQuery(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/schedules" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"schedules":[]}`))
	}))
	t.Cleanup(apiServer.Close)

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
							Name:      builtinContextReadToolName,
							Arguments: `{"resource":"schedules","action":"list"}`,
						},
					},
				},
				nil,
			},
		},
	}

	agentSvc := New(Config{
		Model:                      "test-model",
		LocalAPIBaseURL:            apiServer.URL,
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
	if len(messages[0].ToolCalls) != 1 || messages[0].ToolCalls[0].Name != builtinContextReadToolName {
		t.Fatalf("expected one persisted tool call, got %+v", messages[0].ToolCalls)
	}
}
