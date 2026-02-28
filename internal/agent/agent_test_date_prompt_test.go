package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"strings"
	"testing"
	"time"
)

func TestHandleUserMessage_AlwaysInjectsCurrentDateContextPrompt(t *testing.T) {
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
	fixedNow := time.Date(2026, 2, 18, 10, 30, 45, 0, time.FixedZone("CST", 8*3600))
	agentSvc.nowFn = func() time.Time { return fixedNow }

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

	found := false
	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role != "system" || !strings.Contains(msg.Content, runtimeDateContextMarker) {
			continue
		}
		found = true
		if !strings.Contains(msg.Content, "当前日期 2026-02-18") {
			t.Fatalf("expected current date in prompt, got %q", msg.Content)
		}
		if strings.Contains(msg.Content, "Unix 秒:") {
			t.Fatalf("date-level context should avoid unix timestamp for cache stability, got %q", msg.Content)
		}
	}
	if !found {
		t.Fatalf("expected current-date system prompt to be injected")
	}
}

func TestHandleUserMessage_InjectsOnlyOneRuntimeDateUserContext(t *testing.T) {
	store := conversation.NewStore()
	store.Append("user", buildCurrentDateUserContextPrompt(time.Date(2026, 2, 17, 8, 0, 0, 0, time.FixedZone("CST", 8*3600))))
	store.Append("assistant", "old answer")

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
	agentSvc.nowFn = func() time.Time {
		return time.Date(2026, 2, 18, 10, 30, 45, 0, time.FixedZone("CST", 8*3600))
	}

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

	count := 0
	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role != "system" || !strings.Contains(msg.Content, runtimeDateContextMarker) {
			continue
		}
		count++
		if !strings.Contains(msg.Content, "当前日期 2026-02-18") {
			t.Fatalf("expected latest runtime date context, got %q", msg.Content)
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one runtime date user context, got %d", count)
	}
}

func TestHandleUserMessage_UsesPromptProviderCompressionPrompt(t *testing.T) {
	store := conversation.NewStore()
	store.Append("user", "old question")
	store.Append("assistant", "old answer")

	fakeLLM := &mockLLM{responses: map[string][]string{
		"compress_context": {"summary-v1"},
		"chat_reply":       {"ok"},
	}}

	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 2,
		CompressionTriggerChars:    0,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 2,
		MaxToolCallRounds:          2,
		SystemPrompt:               "default-system",
		CompressionSystemPrompt:    "default-compressor",
	}, store, fakeLLM, nil)
	agentSvc.SetPromptProvider(&mockPromptProvider{
		systemPrompt:            "override-system",
		compressionSystemPrompt: "override-compressor",
	})

	if _, err := agentSvc.HandleUserMessage(context.Background(), "new input"); err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if len(fakeLLM.calls) < 1 {
		t.Fatalf("expected at least one llm call")
	}
	if fakeLLM.calls[0].Purpose != "compress_context" {
		t.Fatalf("first call purpose mismatch: %s", fakeLLM.calls[0].Purpose)
	}
	if got := fakeLLM.calls[0].Messages[0].Content; got != "override-compressor" {
		t.Fatalf("expected provider compression prompt, got %q", got)
	}
}

func TestCompressContext_OnlyIncludesUserAssistantDialogue(t *testing.T) {
	store := conversation.NewStore()
	store.Append("system", "system noise should not be compressed")
	store.Append("assistant", "old answer")

	fakeLLM := &mockLLM{responses: map[string][]string{
		"compress_context": {"summary-v1"},
		"chat_reply":       {"ok"},
	}}

	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 2,
		CompressionTriggerChars:    0,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 2,
		MaxToolCallRounds:          2,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)

	if _, err := agentSvc.HandleUserMessage(context.Background(), "new input"); err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if len(fakeLLM.calls) < 1 || fakeLLM.calls[0].Purpose != "compress_context" {
		t.Fatalf("expected first llm call to be compress_context")
	}

	compressUserPrompt := fakeLLM.calls[0].Messages[1].Content
	if strings.Contains(compressUserPrompt, "[system]") {
		t.Fatalf("compress input should not include system role dialogue, got %q", compressUserPrompt)
	}
	if !strings.Contains(compressUserPrompt, "[assistant] old answer") {
		t.Fatalf("compress input should include assistant dialogue, got %q", compressUserPrompt)
	}
	if !strings.Contains(compressUserPrompt, "[user] new input") {
		t.Fatalf("compress input should include user dialogue, got %q", compressUserPrompt)
	}
}

func TestCompressContext_PrunesSummaryOverlapBeforeCompression(t *testing.T) {
	store := conversation.NewStore()
	store.SetSummaryAndTrim(strings.TrimSpace(`
人格与沟通偏好
- 身份：女性，8 年全栈开发经验
- 偏好：输出包含验收标准
`), 50)
	store.Append("assistant", "old answer")

	fakeLLM := &mockLLM{responses: map[string][]string{
		"compress_context": {"summary-v1"},
		"chat_reply":       {"ok"},
	}}

	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 2,
		CompressionTriggerChars:    0,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 2,
		MaxToolCallRounds:          2,
		SystemPrompt:               "你是用户的 AI 数字分身，女性，8 年全栈开发经验，不使用表情符号。",
		CompressionSystemPrompt:    "你是压缩器。",
	}, store, fakeLLM, nil)

	if _, err := agentSvc.HandleUserMessage(context.Background(), "new input"); err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if len(fakeLLM.calls) < 1 || fakeLLM.calls[0].Purpose != "compress_context" {
		t.Fatalf("expected first llm call to be compress_context")
	}

	compressUserPrompt := fakeLLM.calls[0].Messages[1].Content
	if strings.Contains(compressUserPrompt, "女性，8 年全栈开发经验") {
		t.Fatalf("compress input summary should prune duplicated identity, got %q", compressUserPrompt)
	}
	if strings.Contains(compressUserPrompt, "人格与沟通偏好") {
		t.Fatalf("compress input summary should prune identity heading, got %q", compressUserPrompt)
	}
	if !strings.Contains(compressUserPrompt, "输出包含验收标准") {
		t.Fatalf("compress input should keep user-specific preference, got %q", compressUserPrompt)
	}
}
