package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

type mockLLM struct {
	mu        sync.Mutex
	calls     []llm.ChatRequest
	responses map[string][]string
	toolCalls map[string][][]llm.ToolCall
	errors    map[string][]error
}

func (m *mockLLM) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, req)
	if errQueue := m.errors[req.Purpose]; len(errQueue) > 0 {
		nextErr := errQueue[0]
		m.errors[req.Purpose] = errQueue[1:]
		if nextErr != nil {
			return llm.ChatResponse{}, nextErr
		}
	}

	queue := m.responses[req.Purpose]
	if len(queue) == 0 {
		return llm.ChatResponse{Content: "fallback"}, nil
	}
	out := queue[0]
	m.responses[req.Purpose] = queue[1:]
	var toolCalls []llm.ToolCall
	if tcQueue := m.toolCalls[req.Purpose]; len(tcQueue) > 0 {
		toolCalls = tcQueue[0]
		m.toolCalls[req.Purpose] = tcQueue[1:]
	}

	return llm.ChatResponse{Content: out, ToolCalls: toolCalls}, nil
}

type mockTools struct {
	mu       sync.Mutex
	listed   []llm.ToolDefinition
	calls    []llm.ToolCall
	response map[string]string
}

type mockSkills struct {
	prompts    []string
	indexLines []string
	promptByID map[string]string
	upserts    []evolvedSkill
}

func (m *mockSkills) ListEnabledSkillPrompts() []string {
	return m.prompts
}

func (m *mockSkills) ListEnabledSkillIndex() []string {
	if len(m.indexLines) > 0 {
		return m.indexLines
	}
	out := make([]string, 0, len(m.prompts))
	for i, prompt := range m.prompts {
		id := fmt.Sprintf("skill-%d", i+1)
		out = append(out, fmt.Sprintf("skill_id=%s | name=%s | brief=%s", id, id, prompt))
	}
	return out
}

func (m *mockSkills) ReadEnabledSkillPrompt(skillID string) (string, bool) {
	if m.promptByID == nil {
		return "", false
	}
	prompt, ok := m.promptByID[strings.TrimSpace(skillID)]
	return prompt, ok && strings.TrimSpace(prompt) != ""
}

func (m *mockSkills) UpsertAutoSkill(name, prompt string) error {
	m.upserts = append(m.upserts, evolvedSkill{
		Name:   strings.TrimSpace(name),
		Prompt: strings.TrimSpace(prompt),
	})
	return nil
}

type mockPromptProvider struct {
	systemPrompt            string
	compressionSystemPrompt string
}

func (m *mockPromptProvider) GetSystemPrompt() string {
	return m.systemPrompt
}

func (m *mockPromptProvider) GetCompressionSystemPrompt() string {
	return m.compressionSystemPrompt
}

type mockPromptUpdater struct {
	systemPrompt            string
	compressionSystemPrompt string
	calls                   int
}

func (m *mockPromptUpdater) UpdateAgentPrompts(systemPrompt, compressionSystemPrompt string) error {
	m.systemPrompt = systemPrompt
	m.compressionSystemPrompt = compressionSystemPrompt
	m.calls++
	return nil
}

type mockHabits struct {
	lastSleepReviewDate     string
	lastWakePlanDate        string
	lastPromptEvolutionDate string
}

func (m *mockHabits) GetLastSleepReviewDate() string {
	return m.lastSleepReviewDate
}

func (m *mockHabits) GetLastWakePlanDate() string {
	return m.lastWakePlanDate
}

func (m *mockHabits) GetLastPromptEvolutionDate() string {
	return m.lastPromptEvolutionDate
}

func (m *mockHabits) SetLastSleepReviewDate(date string) error {
	m.lastSleepReviewDate = date
	return nil
}

func (m *mockHabits) SetLastWakePlanDate(date string) error {
	m.lastWakePlanDate = date
	return nil
}

func (m *mockHabits) SetLastPromptEvolutionDate(date string) error {
	m.lastPromptEvolutionDate = date
	return nil
}

func (m *mockTools) ListTools(_ context.Context) ([]llm.ToolDefinition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listed, nil
}

func (m *mockTools) CallTool(_ context.Context, call llm.ToolCall) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, call)
	key := call.Function.Name + ":" + call.Function.Arguments
	if out, ok := m.response[key]; ok {
		return out, nil
	}
	return "{}", nil
}

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
		CompressionTriggerMessages: 3,
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

func TestHandleUserMessage_WithoutCompression(t *testing.T) {
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
}

func TestHandleUserMessage_WithToolCalls(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "weather ready"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "call_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      "weather__query",
							Arguments: `{"city":"beijing"}`,
						},
					},
				},
				nil,
			},
		},
	}
	fakeTools := &mockTools{
		listed: []llm.ToolDefinition{
			{
				Type: "function",
				Function: llm.ToolFunctionDefinition{
					Name: "weather__query",
				},
			},
		},
		response: map[string]string{
			`weather__query:{"city":"beijing"}`: `{"temp":18}`,
		},
	}

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
	}, store, fakeLLM, fakeTools)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "今天北京天气")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "weather ready" {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}
	if len(fakeLLM.calls[0].Tools) != 1 {
		t.Fatalf("expected tools to be passed to llm")
	}
	if len(fakeTools.calls) != 1 || fakeTools.calls[0].Function.Name != "weather__query" {
		t.Fatalf("unexpected tool calls: %+v", fakeTools.calls)
	}
	_, messages := store.Snapshot()
	if len(messages) != 2 {
		t.Fatalf("expected user + assistant messages, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 1 {
		t.Fatalf("expected tool calls attached to user message, got %d", len(messages[0].ToolCalls))
	}
	if messages[0].ToolCalls[0].Name != "weather__query" {
		t.Fatalf("unexpected attached tool name: %s", messages[0].ToolCalls[0].Name)
	}
}

func TestHandleUserMessage_IncludesEnabledSkillPrompts(t *testing.T) {
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
	agentSvc.SetSkillProvider(&mockSkills{
		prompts: []string{"先检索再回答，并提供引用链接。"},
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

	msgs := fakeLLM.calls[0].Messages
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}
	if msgs[1].Role != "system" {
		t.Fatalf("expected second message is system prompt for skills, got %s", msgs[1].Role)
	}
	if !strings.Contains(msgs[1].Content, "先检索再回答") {
		t.Fatalf("skill prompt not injected: %q", msgs[1].Content)
	}
}

func TestHandleUserMessage_SkillCatalogReadViaBuiltinTool(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "已读取技能并给出执行方案"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "call_skill_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      builtinSkillReadToolName,
							Arguments: `{"skill_id":"code-review-playbook"}`,
						},
					},
				},
				nil,
			},
		},
	}
	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          3,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)
	agentSvc.SetSkillProvider(&mockSkills{
		indexLines: []string{
			"skill_id=code-review-playbook | name=代码评审手册 | brief=先确认验收标准，再检查风险与回滚。",
		},
		promptByID: map[string]string{
			"code-review-playbook": "完整技能：先核对需求与验收口径；逐项评审改动风险；输出阻塞与可上线建议。",
		},
	})

	reply, err := agentSvc.HandleUserMessage(context.Background(), "帮我做一次代码评审")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "已读取技能并给出执行方案" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}
	firstCall := fakeLLM.calls[0]
	if len(firstCall.Tools) == 0 {
		t.Fatalf("expected builtin skill read tool to be exposed")
	}
	foundBuiltinTool := false
	for _, tool := range firstCall.Tools {
		if tool.Function.Name == builtinSkillReadToolName {
			foundBuiltinTool = true
			break
		}
	}
	if !foundBuiltinTool {
		t.Fatalf("expected builtin skill read tool in tool list")
	}
	if !strings.Contains(firstCall.Messages[1].Content, "技能索引") {
		t.Fatalf("expected skill index system message, got %q", firstCall.Messages[1].Content)
	}
	secondCall := fakeLLM.calls[1]
	hasToolResponse := false
	for _, msg := range secondCall.Messages {
		if msg.Role == "tool" && strings.Contains(msg.Content, "full_prompt") {
			hasToolResponse = true
			break
		}
	}
	if !hasToolResponse {
		t.Fatalf("expected tool response containing full skill prompt in second llm call")
	}
	_, messages := store.Snapshot()
	if len(messages) != 2 {
		t.Fatalf("expected user + assistant messages, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 1 {
		t.Fatalf("expected skill tool call recorded on user message, got %d", len(messages[0].ToolCalls))
	}
	if messages[0].ToolCalls[0].Name != builtinSkillReadToolName {
		t.Fatalf("unexpected tool call name: %s", messages[0].ToolCalls[0].Name)
	}
}

func TestHandleUserMessage_SkillPromptInjectionIsCapped(t *testing.T) {
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

	content := msgs[1].Content
	if strings.Contains(content, longPrompt) {
		t.Fatalf("expected long prompt to be trimmed out from injected skills")
	}
	if strings.Count(content, "\n") > maxInjectedSkillPrompts+4 {
		t.Fatalf("expected injected skill list to be capped, got: %q", content)
	}
	if !strings.Contains(content, "控制上下文长度") {
		t.Fatalf("expected truncation note, got %q", content)
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
		CompressionTriggerMessages: 3,
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

func TestHandleUserMessage_SleepWindowNonUrgentBypassesLLM(t *testing.T) {
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
	if !strings.Contains(reply, "休息时段") {
		t.Fatalf("expected sleep-window reply, got %q", reply)
	}
	if len(fakeLLM.calls) != 0 {
		t.Fatalf("expected no llm calls in sleep-window non-urgent path, got %d", len(fakeLLM.calls))
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

func TestHandleUserMessage_SleepWindowRunsReflectionAndPromptEvolution(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"night_reflection_evolution": {`{"reflection":"生活：按时休息。工作：推进核心任务。学习：补齐短板。","system_prompt":"你是用户的 AI 数字分身，名字叫“傻毛”，女性，8 年全栈开发经验。你始终不使用表情符号，回答务实、可执行、可复盘，并持续优化工作和学习策略。","compression_system_prompt":"你是“傻毛”数字分身的上下文压缩器，保留人格、事实、任务进度、学习进展与待办，输出简洁纯文本。","skills":[{"name":"故障复盘模板","prompt":"先写事实时间线，再写根因、影响、修复与预防项。"},{"name":"学习闭环","prompt":"每天结束前记录一个短板与一个可执行练习。"}]}`},
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
	agentSvc.SetSkillProvider(skills)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "帮我明天继续优化服务")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if !strings.Contains(reply, "夜间复盘") {
		t.Fatalf("expected reflection section in sleep reply, got %q", reply)
	}
	if updater.calls != 1 {
		t.Fatalf("expected one prompt evolution update, got %d", updater.calls)
	}
	if habits.lastSleepReviewDate != "2026-02-14" {
		t.Fatalf("expected sleep review date recorded, got %q", habits.lastSleepReviewDate)
	}
	if habits.lastPromptEvolutionDate != "2026-02-14" {
		t.Fatalf("expected prompt evolution date recorded, got %q", habits.lastPromptEvolutionDate)
	}
	if len(skills.upserts) != 2 {
		t.Fatalf("expected 2 evolved skills, got %d", len(skills.upserts))
	}
	if skills.upserts[0].Name != "故障复盘模板" {
		t.Fatalf("unexpected first evolved skill: %+v", skills.upserts[0])
	}
}

func TestHandleUserMessage_MorningPlanningPrependsReplyAndTracksDate(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"morning_planning": {"回顾：昨天完成 2 项，1 项待推进。\n今日 Top3：A/B/C。\n能力提升：复盘一个线上问题。"},
		"chat_reply":       {"好的，我先从任务 A 开始。"},
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

	reply, err := agentSvc.HandleUserMessage(context.Background(), "今天我应该先做什么")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if !strings.Contains(reply, "晨间规划") {
		t.Fatalf("expected morning planning prefix in reply, got %q", reply)
	}
	if habits.lastWakePlanDate != "2026-02-14" {
		t.Fatalf("expected wake plan date recorded, got %q", habits.lastWakePlanDate)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected two llm calls (planning + reply), got %d", len(fakeLLM.calls))
	}
	if fakeLLM.calls[0].Purpose != "morning_planning" {
		t.Fatalf("expected first call is morning planning, got %s", fakeLLM.calls[0].Purpose)
	}
}

func TestRunScheduledHumanRoutine_NightReviewAppendsOncePerDay(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"night_reflection_evolution": {`{"reflection":"生活：收束。工作：复盘。学习：迭代。","system_prompt":"你是用户的 AI 数字分身，名字叫“傻毛”，女性，8 年全栈开发经验。你始终不使用表情符号，并保持务实稳定。","compression_system_prompt":"你是“傻毛”数字分身的上下文压缩器，保留人格事实与进度，输出纯文本。"}`},
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

	if err := agentSvc.RunScheduledHumanRoutine(context.Background()); err != nil {
		t.Fatalf("RunScheduledHumanRoutine error: %v", err)
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

	if err := agentSvc.RunScheduledHumanRoutine(context.Background()); err != nil {
		t.Fatalf("RunScheduledHumanRoutine second call error: %v", err)
	}
	_, messages = store.Snapshot()
	if len(messages) != 1 {
		t.Fatalf("expected no duplicate nightly message, got %d", len(messages))
	}
}

func TestRunScheduledHumanRoutine_MorningPlanAppendsOncePerDay(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"morning_planning": {"回顾：昨日 2/3 完成。\n今日 Top3：A/B/C。\n能力提升：复盘线上问题。"},
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

	if err := agentSvc.RunScheduledHumanRoutine(context.Background()); err != nil {
		t.Fatalf("RunScheduledHumanRoutine error: %v", err)
	}
	_, messages := store.Snapshot()
	if len(messages) != 1 {
		t.Fatalf("expected one auto message, got %d", len(messages))
	}
	if !strings.Contains(messages[0].Content, "晨间规划（自动）") {
		t.Fatalf("unexpected auto message: %q", messages[0].Content)
	}

	if err := agentSvc.RunScheduledHumanRoutine(context.Background()); err != nil {
		t.Fatalf("RunScheduledHumanRoutine second call error: %v", err)
	}
	_, messages = store.Snapshot()
	if len(messages) != 1 {
		t.Fatalf("expected no duplicate morning message, got %d", len(messages))
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

func TestRetryLastUserMessage_SleepWindowNonUrgentBypassesLLM(t *testing.T) {
	store := conversation.NewStore()
	store.Append("user", "帮我规划一下明天任务")
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
		EnforceHumanRoutine:        true,
	}, store, fakeLLM, nil)
	agentSvc.nowFn = func() time.Time {
		return time.Date(2026, 2, 14, 3, 0, 0, 0, time.Local)
	}

	reply, err := agentSvc.RetryLastUserMessage(context.Background())
	if err != nil {
		t.Fatalf("RetryLastUserMessage error: %v", err)
	}
	if !strings.Contains(reply, "休息时段") {
		t.Fatalf("expected sleep-window reply, got %q", reply)
	}
	if len(fakeLLM.calls) != 0 {
		t.Fatalf("expected no llm calls, got %d", len(fakeLLM.calls))
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
