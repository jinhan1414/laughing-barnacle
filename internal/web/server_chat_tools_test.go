package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

type mockChatToolLLM struct {
	calls    []llm.ChatRequest
	response string
	err      error
}

func (m *mockChatToolLLM) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.calls = append(m.calls, req)
	if m.err != nil {
		return llm.ChatResponse{}, m.err
	}
	return llm.ChatResponse{Content: m.response}, nil
}

func TestHandleAPIChatToolRun_ContextCompressSuccess(t *testing.T) {
	convStore := conversation.NewStore()
	convStore.Append("user", "u1")
	convStore.Append("assistant", "a1")

	fakeLLM := &mockChatToolLLM{response: "summary-from-tool"}
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
	}, convStore, fakeLLM, nil)

	s := &Server{
		agent:     agentSvc,
		convStore: convStore,
	}

	body := bytes.NewBufferString(`{"tool":"context_compress"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/chat/tools/run", body)
	rec := httptest.NewRecorder()
	s.handleAPIChatToolRun(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload apiChatToolRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v body=%s", err, rec.Body.String())
	}
	if !payload.OK || !payload.Compressed {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Tool != "context_compress" {
		t.Fatalf("unexpected tool: %q", payload.Tool)
	}

	summary, _, events := convStore.SnapshotWithEvents()
	if summary != "summary-from-tool" {
		t.Fatalf("unexpected summary: %q", summary)
	}
	if len(events) != 1 || events[0].Type != "context_compression" {
		t.Fatalf("expected one context_compression event, got %+v", events)
	}
	if len(fakeLLM.calls) != 1 || fakeLLM.calls[0].Purpose != "compress_context" {
		t.Fatalf("expected one compress_context call, got %+v", fakeLLM.calls)
	}
}

func TestHandleAPIChatToolRun_ContextCompressNoop(t *testing.T) {
	convStore := conversation.NewStore()
	fakeLLM := &mockChatToolLLM{response: "summary-from-tool"}
	agentSvc := agent.New(agent.Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 1,
		CompressionTriggerChars:    1,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          2,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, convStore, fakeLLM, nil)

	s := &Server{
		agent:     agentSvc,
		convStore: convStore,
	}

	body := bytes.NewBufferString(`{"tool":"context_compress"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/chat/tools/run", body)
	rec := httptest.NewRecorder()
	s.handleAPIChatToolRun(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload apiChatToolRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v body=%s", err, rec.Body.String())
	}
	if !payload.OK || payload.Compressed {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if len(fakeLLM.calls) != 0 {
		t.Fatalf("expected no llm calls, got %d", len(fakeLLM.calls))
	}
}

func TestHandleAPIChatToolRun_UnknownTool(t *testing.T) {
	s := &Server{
		agent:     &agent.Agent{},
		convStore: conversation.NewStore(),
	}
	body := bytes.NewBufferString(`{"tool":"unknown"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/chat/tools/run", body)
	rec := httptest.NewRecorder()
	s.handleAPIChatToolRun(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}
