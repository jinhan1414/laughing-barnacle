package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"laughing-barnacle/internal/mcp"
)

type mockGreetingLLM struct {
	reply string
	err   error
	calls int
}

func (m *mockGreetingLLM) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.calls++
	if req.Purpose != "chat_greeting" {
		return llm.ChatResponse{}, errors.New("unexpected purpose")
	}
	if m.err != nil {
		return llm.ChatResponse{}, m.err
	}
	return llm.ChatResponse{Content: m.reply}, nil
}

type mockRunningScheduler struct {
	running bool
}

func (m *mockRunningScheduler) Reload() error {
	return nil
}

func (m *mockRunningScheduler) RunNow(_ string) error {
	return nil
}

func (m *mockRunningScheduler) HasRunningTask() bool {
	return m.running
}

func newGreetingTestServer(t *testing.T, llmClient llm.Client) (*Server, *conversation.Store, *mcp.Store) {
	t.Helper()

	convStore := conversation.NewStore()
	mcpStore, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	agentSvc := agent.New(agent.Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          2,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
		EnforceHumanRoutine:        true,
	}, convStore, llmClient, nil)

	return &Server{
		agent:     agentSvc,
		convStore: convStore,
		mcpStore:  mcpStore,
	}, convStore, mcpStore
}

func decodeGreetResponse(t *testing.T, rec *httptest.ResponseRecorder) chatGreetResponse {
	t.Helper()
	var payload chatGreetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v body=%s", err, rec.Body.String())
	}
	return payload
}

func TestHandleChatGreet_FirstVisitCreatesGreeting(t *testing.T) {
	fakeLLM := &mockGreetingLLM{reply: "欢迎回来，我在。"}
	s, convStore, mcpStore := newGreetingTestServer(t, fakeLLM)

	req := httptest.NewRequest(http.MethodPost, "/chat/greet", nil)
	rec := httptest.NewRecorder()
	s.handleChatGreet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeGreetResponse(t, rec)
	if !payload.Created {
		t.Fatalf("expected created=true, got %+v", payload)
	}
	if payload.Content != "欢迎回来，我在。" {
		t.Fatalf("unexpected greeting content: %q", payload.Content)
	}
	if fakeLLM.calls != 1 {
		t.Fatalf("expected one llm call, got %d", fakeLLM.calls)
	}

	_, messages := convStore.Snapshot()
	if len(messages) != 1 || messages[0].Role != "assistant" {
		t.Fatalf("expected one assistant message, got %+v", messages)
	}
	if got := mcpStore.GetLastChatGreetingDate(); got != time.Now().Format("2006-01-02") {
		t.Fatalf("unexpected greeting date persisted: %q", got)
	}
}

func TestHandleChatGreet_SkipWhenCooldown(t *testing.T) {
	fakeLLM := &mockGreetingLLM{reply: "欢迎回来，我在。"}
	s, _, mcpStore := newGreetingTestServer(t, fakeLLM)

	now := time.Now()
	if err := mcpStore.SetLastChatGreetingState(now.Format("2006-01-02"), now, "旧问候"); err != nil {
		t.Fatalf("SetLastChatGreetingState error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/chat/greet", nil)
	rec := httptest.NewRecorder()
	s.handleChatGreet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeGreetResponse(t, rec)
	if payload.Created {
		t.Fatalf("expected created=false, got %+v", payload)
	}
	if payload.Reason != "cooldown" {
		t.Fatalf("unexpected reason: %q", payload.Reason)
	}
	if fakeLLM.calls != 0 {
		t.Fatalf("expected no llm call during cooldown, got %d", fakeLLM.calls)
	}
}

func TestHandleChatGreet_SkipWhenTaskRunning(t *testing.T) {
	fakeLLM := &mockGreetingLLM{reply: "欢迎回来，我在。"}
	s, _, _ := newGreetingTestServer(t, fakeLLM)
	s.scheduler = &mockRunningScheduler{running: true}

	req := httptest.NewRequest(http.MethodPost, "/chat/greet", nil)
	rec := httptest.NewRecorder()
	s.handleChatGreet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeGreetResponse(t, rec)
	if payload.Created {
		t.Fatalf("expected created=false, got %+v", payload)
	}
	if payload.Reason != "task_running" {
		t.Fatalf("unexpected reason: %q", payload.Reason)
	}
	if fakeLLM.calls != 0 {
		t.Fatalf("expected no llm call while task running, got %d", fakeLLM.calls)
	}
}

func TestHandleChatGreet_FallbackWhenLLMFails(t *testing.T) {
	fakeLLM := &mockGreetingLLM{err: errors.New("llm down")}
	s, convStore, _ := newGreetingTestServer(t, fakeLLM)

	req := httptest.NewRequest(http.MethodPost, "/chat/greet", nil)
	rec := httptest.NewRecorder()
	s.handleChatGreet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeGreetResponse(t, rec)
	if !payload.Created {
		t.Fatalf("expected created=true, got %+v", payload)
	}
	if payload.Content == "" {
		t.Fatalf("expected fallback content")
	}

	_, messages := convStore.Snapshot()
	if len(messages) != 1 || messages[0].Content == "" {
		t.Fatalf("expected fallback message appended, got %+v", messages)
	}
}
