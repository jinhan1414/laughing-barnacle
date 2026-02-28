package agent

import (
	"context"
	"strings"
	"testing"

	"laughing-barnacle/internal/conversation"
)

func TestHandleUserMessage_ResponseStrategyInjectedOnce(t *testing.T) {
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
		SystemPrompt:               "## Response Strategy\n- 默认简洁直答（3-6 行）\n- 仅当用户明确要求时再展开",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)

	if _, err := agentSvc.HandleUserMessage(context.Background(), "hello"); err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}

	count := 0
	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role != "system" {
			continue
		}
		if strings.Contains(msg.Content, "默认简洁直答（3-6 行）") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected response strategy to appear once, got %d", count)
	}
}
