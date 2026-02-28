package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"strings"
	"testing"
	"time"
)

func TestHandleUserMessage_SkillIndexInjectionIncludesAllLines(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"chat_reply": {"ok"},
	}}

	longPrompt := strings.Repeat("这个超长技能用于测试提示词裁剪能力。", 40)
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
	agentSvc.SetSkillProvider(&mockSkills{
		prompts: []string{
			"代码变更前先明确验收标准，再拆分实现步骤。",
			"线上故障处理先止血，再定位根因，再补监控与预案。",
			"学习计划采用每天 30 分钟持续练习并复盘。",
			"回答技术方案时给出风险、回滚与验证步骤。",
			"写 SQL 前先确认索引与数据规模。",
			"接口设计优先保证幂等性和可观测性。",
			"发布前执行最小回归用例并记录结果。",
			longPrompt,
		},
	})

	reply, err := agentSvc.HandleUserMessage(context.Background(), "今天要做代码评审并准备上线")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %s", reply)
	}

	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}
	msgs := fakeLLM.calls[0].Messages
	if len(msgs) < 2 {
		t.Fatalf("expected skill system message")
	}

	content := ""
	for _, msg := range msgs {
		if msg.Role == "system" && strings.Contains(msg.Content, "Skills 索引 (共") {
			content = msg.Content
			break
		}
	}
	if strings.TrimSpace(content) == "" {
		t.Fatalf("expected injected skill message")
	}
	if strings.Contains(content, longPrompt) {
		t.Fatalf("expected long prompt to be trimmed out from injected skill index")
	}
	if got := strings.Count(content, "skill_id="); got != 8 {
		t.Fatalf("expected all skill index lines to be injected, got %d lines: %q", got, content)
	}
}

func TestHandleUserMessage_MemoryIndexInjectionIncludesReadHint(t *testing.T) {
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
	agentSvc.SetMemoryProvider(&mockMemory{
		indexLines: []string{
			"path=/projects/pay-refactor/overview | title=支付重构 | summary=灰度中 | rev=3",
		},
	})

	reply, err := agentSvc.HandleUserMessage(context.Background(), "今天支付项目进展如何")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}

	content := ""
	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "MemoryFS 记忆索引（渐进式披露）") {
			content = msg.Content
			break
		}
	}
	if strings.TrimSpace(content) == "" {
		t.Fatalf("expected injected memory index message")
	}
	if !strings.Contains(content, "context__read(resource=\"memory\", action=\"read\", path=\"<path>\")") {
		t.Fatalf("expected memory read hint, got %q", content)
	}
	if !strings.Contains(content, "path=/projects/pay-refactor/overview") {
		t.Fatalf("expected memory index line, got %q", content)
	}
}

func TestHandleUserMessage_UsesPromptProviderSystemPrompt(t *testing.T) {
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
		SystemPrompt:               "default-system",
		CompressionSystemPrompt:    "default-compressor",
	}, store, fakeLLM, nil)
	agentSvc.SetPromptProvider(&mockPromptProvider{
		systemPrompt:            "override-system",
		compressionSystemPrompt: "override-compressor",
	})

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
	if got := fakeLLM.calls[0].Messages[0].Content; got != "override-system" {
		t.Fatalf("expected provider system prompt, got %q", got)
	}
}

func TestHandleUserMessage_IncludesCurrentTimeContextPrompt(t *testing.T) {
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

	reply, err := agentSvc.HandleUserMessage(context.Background(), "查一下最近7天数据")
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
			t.Fatalf("day-level context should avoid unix timestamp for cache stability, got %q", msg.Content)
		}
		if !strings.Contains(msg.Content, "时区: CST") {
			t.Fatalf("expected timezone in prompt, got %q", msg.Content)
		}
	}
	if !found {
		t.Fatalf("expected current-time system prompt to be injected")
	}
}
