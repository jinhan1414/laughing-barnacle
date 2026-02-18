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
	"laughing-barnacle/internal/routine"
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
	if len(fakeLLM.calls[0].Tools) < 2 {
		t.Fatalf("expected builtin bash + external tools, got %d", len(fakeLLM.calls[0].Tools))
	}
	foundBash := false
	foundWeather := false
	for _, tool := range fakeLLM.calls[0].Tools {
		if tool.Function.Name == builtinLinuxBashToolName {
			foundBash = true
		}
		if tool.Function.Name == "weather__query" {
			foundWeather = true
		}
	}
	if !foundBash || !foundWeather {
		t.Fatalf("expected both linux__bash and weather__query tools, got %+v", fakeLLM.calls[0].Tools)
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

func TestHandleUserMessage_RequiresToolEvidenceForRuntimeScheduleQuery(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"当前有 2 个定时任务。", "已经查好了。"},
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

	_, err := agentSvc.HandleUserMessage(context.Background(), "有哪些定时任务")
	if err == nil {
		t.Fatalf("expected runtime-evidence error")
	}
	if !strings.Contains(err.Error(), "需要先调用工具") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected two llm calls due evidence enforcement, got %d", len(fakeLLM.calls))
	}

	_, messages := store.Snapshot()
	if len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("expected only pending user message, got %+v", messages)
	}
}

func TestHandleUserMessage_IncludesSkillIndexForProgressiveDisclosure(t *testing.T) {
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
		indexLines: []string{
			"skill_id=research-flow | name=检索闭环 | description=先检索再回答 | brief=检索后再回答并附来源 | path=skill://research-flow/SKILL.md",
		},
		promptByID: map[string]string{
			"research-flow": "这是完整技能正文，不应在首轮直接注入。",
		},
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
	content := ""
	for _, msg := range msgs {
		if msg.Role == "system" && strings.Contains(msg.Content, "已启用技能索引（渐进式披露）") {
			content = msg.Content
			break
		}
	}
	if strings.TrimSpace(content) == "" {
		t.Fatalf("skill index not injected")
	}
	if !strings.Contains(content, "/api/skills/read?id=<skill_id>") {
		t.Fatalf("expected bash curl read hint, got %q", content)
	}
	if strings.Contains(content, "本轮高相关候选") {
		t.Fatalf("unexpected related-skill hint for unrelated query, got %q", content)
	}
	if strings.Contains(content, "完整技能正文") {
		t.Fatalf("should not inject full skill content directly, got %q", content)
	}
	for _, msg := range msgs {
		if msg.Role != "system" {
			continue
		}
		if strings.Contains(msg.Content, "/api/context/archive/index") || strings.Contains(msg.Content, "内置技能 archive_recall") {
			t.Fatalf("archive recall should not be hardcoded in system prompt, got %q", msg.Content)
		}
	}
}

func TestHandleUserMessage_UsesOnlyBuiltinLinuxBashTool(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"done"},
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
	reply, err := agentSvc.HandleUserMessage(context.Background(), "test")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "done" {
		t.Fatalf("unexpected reply: %q", reply)
	}

	firstCall := fakeLLM.calls[0]
	if len(firstCall.Tools) != 1 {
		t.Fatalf("expected exactly one builtin tool, got %d", len(firstCall.Tools))
	}
	for _, tool := range firstCall.Tools {
		if tool.Function.Name != builtinLinuxBashToolName {
			t.Fatalf("unexpected builtin tool: %s", tool.Function.Name)
		}
	}
}

func TestHandleUserMessage_LinuxBashToolCall(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "bash ok"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "call_bash_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      builtinLinuxBashToolName,
							Arguments: `{"command":"echo hello-linux-bash"}`,
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

	reply, err := agentSvc.HandleUserMessage(context.Background(), "run bash")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "bash ok" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}

	secondCall := fakeLLM.calls[1]
	foundToolResult := false
	for _, msg := range secondCall.Messages {
		if msg.Role == "tool" && strings.Contains(msg.Content, "exit_code: 0") && strings.Contains(msg.Content, "hello-linux-bash") {
			foundToolResult = true
			break
		}
	}
	if !foundToolResult {
		t.Fatalf("expected linux__bash tool result in second call")
	}

	_, messages := store.Snapshot()
	if len(messages) != 2 {
		t.Fatalf("expected user + assistant messages, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 1 {
		t.Fatalf("expected one tool call recorded, got %d", len(messages[0].ToolCalls))
	}
	if messages[0].ToolCalls[0].Name != builtinLinuxBashToolName {
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

	content := ""
	for _, msg := range msgs {
		if msg.Role == "system" && strings.Contains(msg.Content, "已启用技能索引（渐进式披露）") {
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
	if strings.Count(content, "\n") > maxInjectedSkillPrompts+4 {
		t.Fatalf("expected injected skill list to be capped, got: %q", content)
	}
	if !strings.Contains(content, "索引共") {
		t.Fatalf("expected index truncation note, got %q", content)
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
	agentSvc.SetSkillProvider(&mockSkills{
		promptByID: map[string]string{
			routine.ScheduledSkillNightReflectionEvolution: "---\nname: \"夜间复盘\"\ndescription: \"night\"\n---\n\n执行夜间复盘",
		},
	})

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
	agentSvc.SetSkillProvider(&mockSkills{
		promptByID: map[string]string{
			routine.ScheduledSkillMorningPlanning: "---\nname: \"晨间规划\"\ndescription: \"morning\"\n---\n\n执行晨间规划",
		},
	})

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
