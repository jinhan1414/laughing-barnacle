package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

type mockLLM struct {
	mu        sync.Mutex
	calls     []llm.ChatRequest
	responses map[string][]string
	toolCalls map[string][][]llm.ToolCall
	errors    map[string][]error
}

func (m *mockLLM) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, req)
	if errQueue := m.errors[req.Purpose]; len(errQueue) > 0 {
		nextErr := errQueue[0]
		m.errors[req.Purpose] = errQueue[1:]
		if nextErr != nil {
			return llm.ChatResponse{}, nextErr
		}
	}

	queue := m.responses[req.Purpose]
	if len(queue) == 0 {
		return llm.ChatResponse{Content: "fallback"}, nil
	}
	out := queue[0]
	m.responses[req.Purpose] = queue[1:]
	var toolCalls []llm.ToolCall
	if tcQueue := m.toolCalls[req.Purpose]; len(tcQueue) > 0 {
		toolCalls = tcQueue[0]
		m.toolCalls[req.Purpose] = tcQueue[1:]
	}

	return llm.ChatResponse{Content: out, ToolCalls: toolCalls}, nil
}

type mockTools struct {
	mu       sync.Mutex
	listed   []llm.ToolDefinition
	calls    []llm.ToolCall
	response map[string]string
}

type mockSkills struct {
	prompts []string
}

func (m *mockSkills) ListEnabledSkillPrompts() []string {
	return m.prompts
}

func (m *mockTools) ListTools(_ context.Context) ([]llm.ToolDefinition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listed, nil
}

func (m *mockTools) CallTool(_ context.Context, call llm.ToolCall) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, call)
	key := call.Function.Name + ":" + call.Function.Arguments
	if out, ok := m.response[key]; ok {
		return out, nil
	}
	return "{}", nil
}

func TestHandleUserMessage_WithAutoCompression(t *testing.T) {
	store := conversation.NewStore()
	store.Append("user", "old question")
	store.Append("assistant", "old answer")

	fakeLLM := &mockLLM{responses: map[string][]string{
		"compress_context": {"summary-v1"},
		"chat_reply":       {"final-answer"},
	}}

	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 3,
		CompressionTriggerChars:    0,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 2,
		MaxToolCallRounds:          4,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "new input")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "final-answer" {
		t.Fatalf("unexpected reply: %s", reply)
	}

	summary, messages := store.Snapshot()
	if summary != "summary-v1" {
		t.Fatalf("summary not updated, got %q", summary)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages after trim + reply, got %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "new input" {
		t.Fatalf("unexpected first message: %+v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "final-answer" {
		t.Fatalf("unexpected second message: %+v", messages[1])
	}

	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}
	if fakeLLM.calls[0].Purpose != "compress_context" {
		t.Fatalf("first call purpose mismatch: %s", fakeLLM.calls[0].Purpose)
	}
	if fakeLLM.calls[1].Purpose != "chat_reply" {
		t.Fatalf("second call purpose mismatch: %s", fakeLLM.calls[1].Purpose)
	}
}

func TestHandleUserMessage_WithoutCompression(t *testing.T) {
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
		MaxToolCallRounds:          4,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(fakeLLM.calls) != 1 || fakeLLM.calls[0].Purpose != "chat_reply" {
		t.Fatalf("unexpected calls: %+v", fakeLLM.calls)
	}
}

func TestHandleUserMessage_WithToolCalls(t *testing.T) {
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
	if len(fakeLLM.calls[0].Tools) != 1 {
		t.Fatalf("expected tools to be passed to llm")
	}
	if len(fakeTools.calls) != 1 || fakeTools.calls[0].Function.Name != "weather__query" {
		t.Fatalf("unexpected tool calls: %+v", fakeTools.calls)
	}
}

func TestHandleUserMessage_IncludesEnabledSkillPrompts(t *testing.T) {
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
	agentSvc.SetSkillProvider(&mockSkills{
		prompts: []string{"先检索再回答，并提供引用链接。"},
	})

	reply, err := agentSvc.HandleUserMessage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}

	msgs := fakeLLM.calls[0].Messages
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}
	if msgs[1].Role != "system" {
		t.Fatalf("expected second message is system prompt for skills, got %s", msgs[1].Role)
	}
	if !strings.Contains(msgs[1].Content, "先检索再回答") {
		t.Fatalf("skill prompt not injected: %q", msgs[1].Content)
	}
}

func TestRetryLastUserMessage_ReusesPendingUserMessage(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"retry-ok"},
		},
		errors: map[string][]error{
			"chat_reply": {errors.New("llm unavailable"), nil},
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

	if _, err := agentSvc.HandleUserMessage(context.Background(), "hello"); err == nil {
		t.Fatalf("expected first chat to fail")
	}

	_, messages := store.Snapshot()
	if len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("expected only pending user message, got %+v", messages)
	}

	reply, err := agentSvc.RetryLastUserMessage(context.Background())
	if err != nil {
		t.Fatalf("RetryLastUserMessage error: %v", err)
	}
	if reply != "retry-ok" {
		t.Fatalf("unexpected retry reply: %s", reply)
	}

	_, messages = store.Snapshot()
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages after retry, got %d", len(messages))
	}
	if messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("unexpected roles after retry: %+v", messages)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}
}

func TestRetryLastUserMessage_NoPendingUser(t *testing.T) {
	store := conversation.NewStore()
	store.Append("assistant", "ready")
	fakeLLM := &mockLLM{}

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

	if _, err := agentSvc.RetryLastUserMessage(context.Background()); err == nil {
		t.Fatalf("expected retry to fail when no pending user message")
	}
}
