package agent

import (
	"context"
	"errors"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/routine"
	"strings"
	"testing"
	"time"
)

func TestHandleUserMessage_DoesNotPrependMorningPlanning(t *testing.T) {
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
	habits := &mockHabits{}
	agentSvc.SetHabitProvider(habits)
	agentSvc.SetSkillProvider(&mockSkills{
		promptByID: map[string]string{
			routine.ScheduledSkillMorningPlanning: "---\nname: \"晨间规划\"\ndescription: \"morning\"\n---\n\n执行晨间规划",
		},
	})

	reply, err := agentSvc.HandleUserMessage(context.Background(), "今天我应该先做什么")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "好的，我先从任务 A 开始。" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if habits.lastWakePlanDate != "" {
		t.Fatalf("expected no wake plan date update in message path, got %q", habits.lastWakePlanDate)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call (chat only), got %d", len(fakeLLM.calls))
	}
	if fakeLLM.calls[0].Purpose != "chat_reply" {
		t.Fatalf("expected chat reply call, got %s", fakeLLM.calls[0].Purpose)
	}
}

func TestRunScheduledTask_NightReviewAppendsOncePerDay(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"scheduled_skill_night_reflection_evolution": {`{"reflection":"生活：收束。工作：复盘。学习：迭代。","system_prompt":"你是用户的 AI 数字分身，名字叫“傻毛”，女性，8 年全栈开发经验。你始终不使用表情符号，并保持务实稳定。","compression_system_prompt":"你是“傻毛”数字分身的上下文压缩器，保留人格事实与进度，输出纯文本。"}`},
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
		return time.Date(2026, 2, 14, 2, 30, 0, 0, time.Local)
	}
	updater := &mockPromptUpdater{}
	habits := &mockHabits{}
	agentSvc.SetPromptUpdater(updater)
	agentSvc.SetHabitProvider(habits)
	agentSvc.SetSkillProvider(&mockSkills{
		promptByID: map[string]string{
			routine.ScheduledSkillNightReflectionEvolution: "---\nname: \"夜间复盘\"\ndescription: \"night\"\n---\n\n执行夜间复盘",
		},
	})

	if err := agentSvc.RunScheduledTask(context.Background(), routine.ActionNightReflectionEvolution); err != nil {
		t.Fatalf("RunScheduledTask error: %v", err)
	}
	_, messages := store.Snapshot()
	if len(messages) != 1 {
		t.Fatalf("expected one auto message, got %d", len(messages))
	}
	if !strings.Contains(messages[0].Content, "夜间复盘（自动）") {
		t.Fatalf("unexpected auto message: %q", messages[0].Content)
	}
	if updater.calls != 1 {
		t.Fatalf("expected one prompt update, got %d", updater.calls)
	}

	if err := agentSvc.RunScheduledTask(context.Background(), routine.ActionNightReflectionEvolution); err != nil {
		t.Fatalf("RunScheduledTask second call error: %v", err)
	}
	_, messages = store.Snapshot()
	if len(messages) != 1 {
		t.Fatalf("expected no duplicate nightly message, got %d", len(messages))
	}
}

func TestRunScheduledTask_MorningPlanAppendsOncePerDay(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"scheduled_skill_morning_planning": {"回顾：昨日 2/3 完成。\n今日 Top3：A/B/C。\n能力提升：复盘线上问题。"},
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
		return time.Date(2026, 2, 14, 9, 0, 0, 0, time.Local)
	}
	habits := &mockHabits{}
	agentSvc.SetHabitProvider(habits)
	agentSvc.SetSkillProvider(&mockSkills{
		promptByID: map[string]string{
			routine.ScheduledSkillMorningPlanning: "---\nname: \"晨间规划\"\ndescription: \"morning\"\n---\n\n执行晨间规划",
		},
	})

	if err := agentSvc.RunScheduledTask(context.Background(), routine.ActionMorningPlanning); err != nil {
		t.Fatalf("RunScheduledTask error: %v", err)
	}
	_, messages := store.Snapshot()
	if len(messages) != 1 {
		t.Fatalf("expected one auto message, got %d", len(messages))
	}
	if !strings.Contains(messages[0].Content, "晨间规划（自动）") {
		t.Fatalf("unexpected auto message: %q", messages[0].Content)
	}

	if err := agentSvc.RunScheduledTask(context.Background(), routine.ActionMorningPlanning); err != nil {
		t.Fatalf("RunScheduledTask second call error: %v", err)
	}
	_, messages = store.Snapshot()
	if len(messages) != 1 {
		t.Fatalf("expected no duplicate morning message, got %d", len(messages))
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
