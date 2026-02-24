package agent

import (
	"context"
	"errors"
	"laughing-barnacle/internal/conversation"
	"strings"
	"testing"
	"time"
)

func TestRetryLastUserMessage_SleepWindowNonUrgentStillCallsLLM(t *testing.T) {
	store := conversation.NewStore()
	store.Append("user", "帮我规划一下明天任务")
	fakeLLM := &mockLLM{responses: map[string][]string{
		"chat_reply": {"retry-ok"},
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
		EnforceHumanRoutine:        true,
	}, store, fakeLLM, nil)
	agentSvc.nowFn = func() time.Time {
		return time.Date(2026, 2, 14, 3, 0, 0, 0, time.Local)
	}

	reply, err := agentSvc.RetryLastUserMessage(context.Background())
	if err != nil {
		t.Fatalf("RetryLastUserMessage error: %v", err)
	}
	if reply != "retry-ok" {
		t.Fatalf("unexpected retry reply: %q", reply)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}
	if fakeLLM.calls[0].Purpose != "chat_reply" {
		t.Fatalf("expected chat reply call, got %s", fakeLLM.calls[0].Purpose)
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

func TestGenerateChatGreeting_UsesContextAndPurpose(t *testing.T) {
	store := conversation.NewStore()
	store.Append("user", "早上好")
	store.Append("assistant", "我在，今天先做哪件事？")
	store.SetSummaryAndTrim("用户正在推进支付链路改造。", 10)

	fakeLLM := &mockLLM{responses: map[string][]string{
		"chat_greeting": {"  欢迎回来，我已就绪，今天先从哪一项开始？  "},
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

	out, err := agentSvc.GenerateChatGreeting(context.Background(), ChatGreetingInput{
		Now:                 time.Date(2026, 2, 17, 9, 0, 0, 0, time.Local),
		IsFirstToday:        true,
		LastGreetingAt:      time.Date(2026, 2, 16, 19, 0, 0, 0, time.Local),
		LastGreetingContent: "晚上好",
		RecentTaskStatuses:  []string{"morning-planning | success"},
	})
	if err != nil {
		t.Fatalf("GenerateChatGreeting error: %v", err)
	}
	if out != "欢迎回来，我已就绪，今天先从哪一项开始？" {
		t.Fatalf("unexpected greeting content: %q", out)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}
	if fakeLLM.calls[0].Purpose != "chat_greeting" {
		t.Fatalf("expected chat_greeting purpose, got %q", fakeLLM.calls[0].Purpose)
	}
	if len(fakeLLM.calls[0].Messages) < 2 {
		t.Fatalf("expected prompt messages, got %d", len(fakeLLM.calls[0].Messages))
	}
	if !strings.Contains(fakeLLM.calls[0].Messages[1].Content, "最近任务状态") {
		t.Fatalf("expected task context in prompt, got %q", fakeLLM.calls[0].Messages[1].Content)
	}
}

func TestGenerateChatGreeting_LLMError(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		errors: map[string][]error{
			"chat_greeting": {errors.New("timeout")},
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

	if _, err := agentSvc.GenerateChatGreeting(context.Background(), ChatGreetingInput{}); err == nil {
		t.Fatalf("expected llm error")
	}
}
