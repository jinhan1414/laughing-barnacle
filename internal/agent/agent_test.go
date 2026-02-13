package agent

import (
	"context"
	"sync"
	"testing"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

type mockLLM struct {
	mu        sync.Mutex
	calls     []llm.ChatRequest
	responses map[string][]string
}

func (m *mockLLM) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, req)
	queue := m.responses[req.Purpose]
	if len(queue) == 0 {
		return llm.ChatResponse{Content: "fallback"}, nil
	}
	out := queue[0]
	m.responses[req.Purpose] = queue[1:]
	return llm.ChatResponse{Content: out}, nil
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
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM)

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
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM)

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
