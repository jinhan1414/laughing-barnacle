package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/routine"
	"strings"
	"testing"
	"time"
)

func TestHandleUserMessage_PrunesSummaryOverlapFromSystemPrompt(t *testing.T) {
	store := conversation.NewStore()
	store.SetSummaryAndTrim(strings.TrimSpace(`
人格与沟通偏好
- 身份：女性，8 年全栈开发经验
- 偏好：回答时给出具体日期

工作进展
- 项目：支付重构
`), 50)

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
		SystemPrompt:               "你是用户的 AI 数字分身，名字叫傻毛，女性，8 年全栈开发经验。不使用表情符号。",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "今天安排")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %s", reply)
	}

	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}
	summaryPayload := ""
	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role == "system" && strings.HasPrefix(msg.Content, "历史摘要（由系统自动压缩）：") {
			summaryPayload = msg.Content
			break
		}
	}
	if strings.TrimSpace(summaryPayload) == "" {
		t.Fatalf("expected summary system message")
	}
	if strings.Contains(summaryPayload, "女性，8 年全栈开发经验") {
		t.Fatalf("expected duplicated identity line to be pruned, got %q", summaryPayload)
	}
	if strings.Contains(summaryPayload, "人格与沟通偏好") {
		t.Fatalf("expected identity heading to be pruned, got %q", summaryPayload)
	}
	if !strings.Contains(summaryPayload, "回答时给出具体日期") {
		t.Fatalf("expected user-specific preference to remain, got %q", summaryPayload)
	}
	if !strings.Contains(summaryPayload, "支付重构") {
		t.Fatalf("expected work-progress fact to remain, got %q", summaryPayload)
	}
}

func TestHandleUserMessage_AutoCompressionPrunesPromptDuplicatesInStoredSummary(t *testing.T) {
	store := conversation.NewStore()
	store.Append("user", "old question")
	store.Append("assistant", "old answer")

	fakeLLM := &mockLLM{responses: map[string][]string{
		"compress_context": {strings.TrimSpace(`
人格与沟通偏好
- 身份：女性，8 年全栈开发经验
- 偏好：输出包含明确验收标准

待办清单
- [P0] 今天补齐回归测试
`)},
		"chat_reply": {"ok"},
	}}

	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 3,
		CompressionTriggerChars:    0,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 2,
		MaxToolCallRounds:          2,
		SystemPrompt:               "你是用户的 AI 数字分身，女性，8 年全栈开发经验，不使用表情符号。",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)

	if _, err := agentSvc.HandleUserMessage(context.Background(), "new input"); err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}

	summary, _ := store.Snapshot()
	if strings.Contains(summary, "女性，8 年全栈开发经验") {
		t.Fatalf("expected duplicated identity line to be pruned from summary, got %q", summary)
	}
	if strings.Contains(summary, "人格与沟通偏好") {
		t.Fatalf("expected identity heading to be pruned from summary, got %q", summary)
	}
	if !strings.Contains(summary, "输出包含明确验收标准") {
		t.Fatalf("expected user-specific preference kept in summary, got %q", summary)
	}
	if !strings.Contains(summary, "今天补齐回归测试") {
		t.Fatalf("expected todo kept in summary, got %q", summary)
	}
}

func TestHandleUserMessage_SleepWindowNonUrgentStillCallsLLM(t *testing.T) {
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
		EnforceHumanRoutine:        true,
	}, store, fakeLLM, nil)
	agentSvc.nowFn = func() time.Time {
		return time.Date(2026, 2, 14, 2, 0, 0, 0, time.Local)
	}

	reply, err := agentSvc.HandleUserMessage(context.Background(), "帮我整理下周学习计划")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call in sleep-window non-urgent path, got %d", len(fakeLLM.calls))
	}
	if fakeLLM.calls[0].Purpose != "chat_reply" {
		t.Fatalf("expected chat reply call, got %s", fakeLLM.calls[0].Purpose)
	}
	_, messages := store.Snapshot()
	if len(messages) != 2 || messages[1].Role != "assistant" {
		t.Fatalf("expected user + assistant messages, got %+v", messages)
	}
}

func TestHandleUserMessage_SleepWindowUrgentStillCallsLLM(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"chat_reply": {"紧急止损方案"},
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
		return time.Date(2026, 2, 14, 2, 0, 0, 0, time.Local)
	}

	reply, err := agentSvc.HandleUserMessage(context.Background(), "紧急：生产环境宕机，马上给我止损方案")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "紧急止损方案" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected llm to be called for urgent message, got %d", len(fakeLLM.calls))
	}
}

func TestHandleUserMessage_SleepWindowDoesNotRunReflectionOrEvolution(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"chat_reply": {"正常回复"},
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
		return time.Date(2026, 2, 14, 2, 10, 0, 0, time.Local)
	}
	updater := &mockPromptUpdater{}
	habits := &mockHabits{}
	skills := &mockSkills{}
	agentSvc.SetPromptUpdater(updater)
	agentSvc.SetHabitProvider(habits)
	agentSvc.SetSkillProvider(&mockSkills{
		promptByID: map[string]string{
			routine.ScheduledSkillNightReflectionEvolution: "---\nname: \"夜间复盘\"\ndescription: \"night\"\n---\n\n执行夜间复盘",
		},
	})
	agentSvc.SetSkillProvider(skills)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "帮我明天继续优化服务")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "正常回复" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if len(fakeLLM.calls) != 1 || fakeLLM.calls[0].Purpose != "chat_reply" {
		t.Fatalf("expected only chat reply call, got %+v", fakeLLM.calls)
	}
	if updater.calls != 0 {
		t.Fatalf("expected no prompt evolution update, got %d", updater.calls)
	}
	if habits.lastSleepReviewDate != "" {
		t.Fatalf("expected no sleep review date update, got %q", habits.lastSleepReviewDate)
	}
	if habits.lastPromptEvolutionDate != "" {
		t.Fatalf("expected no prompt evolution date update, got %q", habits.lastPromptEvolutionDate)
	}
	if len(skills.upserts) != 0 {
		t.Fatalf("expected no evolved skills, got %d", len(skills.upserts))
	}
}
