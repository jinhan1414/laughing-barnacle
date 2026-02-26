package agent

import (
	"context"
	"errors"
	"laughing-barnacle/internal/conversation"
	"strings"
	"testing"
	"time"
)

func TestHandleUserMessage_DoesNotPrependScheduledPlan(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"chat_reply": {"好的，我先从任务 A 开始。"},
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
		return time.Date(2026, 2, 14, 9, 5, 0, 0, time.Local)
	}

	reply, err := agentSvc.HandleUserMessage(context.Background(), "今天我应该先做什么")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "好的，我先从任务 A 开始。" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call (chat only), got %d", len(fakeLLM.calls))
	}
	if fakeLLM.calls[0].Purpose != "chat_reply" {
		t.Fatalf("expected chat reply call, got %s", fakeLLM.calls[0].Purpose)
	}
}

func TestRunScheduledTask_GenericSkillAppendsMessage(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"scheduled_skill_daily_review": {"今日总结：完成核心任务并记录风险。"},
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
	agentSvc.SetSkillProvider(&mockSkills{
		promptByID: map[string]string{
			"daily-review": "---\nname: \"日终复盘\"\ndescription: \"daily review\"\n---\n\n输出今日复盘",
		},
	})

	if err := agentSvc.RunScheduledTask(context.Background(), "skill:daily-review"); err != nil {
		t.Fatalf("RunScheduledTask error: %v", err)
	}
	_, messages := store.Snapshot()
	if len(messages) != 1 {
		t.Fatalf("expected one auto message, got %d", len(messages))
	}
	if !strings.Contains(messages[0].Content, "定时任务（自动）日终复盘") {
		t.Fatalf("unexpected auto message: %q", messages[0].Content)
	}
}

func TestRunScheduledTask_AppendsFailureMessageWhenSkillMissing(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{}}

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

	err := agentSvc.RunScheduledTask(context.Background(), "skill:not-installed")
	if err == nil {
		t.Fatalf("expected scheduled task error")
	}

	_, messages := store.Snapshot()
	if len(messages) != 1 {
		t.Fatalf("expected one failure message, got %d", len(messages))
	}
	if messages[0].Role != "assistant" {
		t.Fatalf("expected assistant failure message, got role=%q", messages[0].Role)
	}
	if !strings.Contains(messages[0].Content, "定时任务执行失败") {
		t.Fatalf("expected failure prefix, got %q", messages[0].Content)
	}
	if !strings.Contains(messages[0].Content, "skill:not-installed") {
		t.Fatalf("expected task action in failure message, got %q", messages[0].Content)
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
		EnforceHumanRoutine:        true,
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
