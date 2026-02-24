package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"strings"
	"testing"
)

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
		CompressionTriggerMessages: 2,
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

func TestCompressContextNow_ForcesCompressionWithoutThreshold(t *testing.T) {
	store := conversation.NewStore()
	store.Append("user", "old question")
	store.Append("assistant", "old answer")

	fakeLLM := &mockLLM{responses: map[string][]string{
		"compress_context": {"manual-summary"},
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

	summary, compressed, err := agentSvc.CompressContextNow(context.Background())
	if err != nil {
		t.Fatalf("CompressContextNow error: %v", err)
	}
	if !compressed {
		t.Fatalf("expected compressed=true")
	}
	if summary != "manual-summary" {
		t.Fatalf("unexpected summary return: %q", summary)
	}
	if len(fakeLLM.calls) != 1 || fakeLLM.calls[0].Purpose != "compress_context" {
		t.Fatalf("expected exactly one compress_context call, got %+v", fakeLLM.calls)
	}

	currentSummary, messages, events := store.SnapshotWithEvents()
	if currentSummary != "manual-summary" {
		t.Fatalf("unexpected store summary: %q", currentSummary)
	}
	if len(messages) != 0 {
		t.Fatalf("expected no remaining messages after manual compression, got %+v", messages)
	}
	if len(events) != 1 || strings.TrimSpace(events[0].Type) != "context_compression" {
		t.Fatalf("expected one context_compression event, got %+v", events)
	}
}

func TestCompressContextNow_NoConversationNoop(t *testing.T) {
	store := conversation.NewStore()

	fakeLLM := &mockLLM{responses: map[string][]string{
		"compress_context": {"manual-summary"},
	}}

	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 1,
		CompressionTriggerChars:    1,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          2,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)

	summary, compressed, err := agentSvc.CompressContextNow(context.Background())
	if err != nil {
		t.Fatalf("CompressContextNow error: %v", err)
	}
	if compressed {
		t.Fatalf("expected compressed=false")
	}
	if summary != "" {
		t.Fatalf("expected empty summary, got %q", summary)
	}
	if len(fakeLLM.calls) != 0 {
		t.Fatalf("expected no llm calls, got %d", len(fakeLLM.calls))
	}
}

func TestHandleUserMessage_WithoutCompression(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"chat_reply": {"ok"},
	}, usages: map[string][]llm.TokenUsage{
		"chat_reply": {
			{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
		},
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
	_, messages := store.Snapshot()
	if len(messages) != 2 || messages[1].Usage == nil || messages[1].Usage.TotalTokens != 12 {
		t.Fatalf("expected assistant usage=12, got %+v", messages)
	}
}
