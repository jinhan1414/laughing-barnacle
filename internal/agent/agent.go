package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

type Config struct {
	Model                      string
	Temperature                float64
	MaxRecentMessages          int
	CompressionTriggerMessages int
	CompressionTriggerChars    int
	KeepRecentAfterCompression int
	MaxCompressionLoopsPerTurn int
	MaxToolCallRounds          int
	SystemPrompt               string
	CompressionSystemPrompt    string
	EnforceHumanRoutine        bool
}

type ToolProvider interface {
	ListTools(ctx context.Context) ([]llm.ToolDefinition, error)
	CallTool(ctx context.Context, call llm.ToolCall) (string, error)
}

type SkillProvider interface {
	ListEnabledSkillPrompts() []string
}

type PromptProvider interface {
	GetSystemPrompt() string
	GetCompressionSystemPrompt() string
}

type PromptUpdater interface {
	UpdateAgentPrompts(systemPrompt, compressionSystemPrompt string) error
}

type HabitProvider interface {
	GetLastSleepReviewDate() string
	GetLastWakePlanDate() string
	GetLastPromptEvolutionDate() string
	SetLastSleepReviewDate(date string) error
	SetLastWakePlanDate(date string) error
	SetLastPromptEvolutionDate(date string) error
}

type Agent struct {
	cfg     Config
	llm     llm.Client
	tools   ToolProvider
	skills  SkillProvider
	prompts PromptProvider
	updater PromptUpdater
	habits  HabitProvider
	store   *conversation.Store
	nowFn   func() time.Time
	mu      sync.Mutex
}

func New(cfg Config, store *conversation.Store, llmClient llm.Client, tools ToolProvider) *Agent {
	return &Agent{
		cfg:   cfg,
		llm:   llmClient,
		tools: tools,
		store: store,
		nowFn: time.Now,
	}
}

func (a *Agent) SetSkillProvider(provider SkillProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.skills = provider
}

func (a *Agent) SetPromptProvider(provider PromptProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prompts = provider
}

func (a *Agent) SetPromptUpdater(updater PromptUpdater) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.updater = updater
}

func (a *Agent) SetHabitProvider(provider HabitProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.habits = provider
}

func (a *Agent) GetEffectivePrompts() (systemPrompt string, compressionSystemPrompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.resolvePromptsLocked()
}

// HandleUserMessage processes one user turn, updating shared conversation state.
func (a *Agent) HandleUserMessage(ctx context.Context, userInput string) (string, error) {
	text := strings.TrimSpace(userInput)
	if text == "" {
		return "", fmt.Errorf("empty input")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.store.Append("user", text)
	now := a.nowFn()
	if a.cfg.EnforceHumanRoutine && shouldEnforceSleepReply(text, now) {
		reflection := strings.TrimSpace(a.runNightReflectionAndEvolution(ctx, now))
		reply := sleepWindowReply()
		if reflection != "" {
			reply = "【夜间复盘】\n" + reflection + "\n\n" + reply
		}
		a.store.Append("assistant", reply)
		return reply, nil
	}
	morningPlan := strings.TrimSpace(a.runMorningPlanning(ctx, now))

	if err := a.autonomousCompressionLoop(ctx); err != nil {
		return "", err
	}

	_, messages := a.store.Snapshot()
	reply, toolCalls, err := a.generateReply(ctx, messages)
	_ = a.store.SetLatestUserToolCalls(toolCalls)
	if err != nil {
		return "", err
	}

	reply = strings.TrimSpace(reply)
	if morningPlan != "" {
		reply = strings.TrimSpace("【晨间规划】\n" + morningPlan + "\n\n" + reply)
	}
	a.store.Append("assistant", reply)
	return reply, nil
}

// RetryLastUserMessage retries generating assistant output for the latest pending user message.
func (a *Agent) RetryLastUserMessage(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	_, messages := a.store.Snapshot()
	if len(messages) == 0 || messages[len(messages)-1].Role != "user" {
		return "", fmt.Errorf("no pending user message to retry")
	}
	pendingUserMessage := messages[len(messages)-1].Content
	now := a.nowFn()
	if a.cfg.EnforceHumanRoutine && shouldEnforceSleepReply(pendingUserMessage, now) {
		reflection := strings.TrimSpace(a.runNightReflectionAndEvolution(ctx, now))
		reply := sleepWindowReply()
		if reflection != "" {
			reply = "【夜间复盘】\n" + reflection + "\n\n" + reply
		}
		a.store.Append("assistant", reply)
		return reply, nil
	}
	morningPlan := strings.TrimSpace(a.runMorningPlanning(ctx, now))

	if err := a.autonomousCompressionLoop(ctx); err != nil {
		return "", err
	}

	_, messages = a.store.Snapshot()
	if len(messages) == 0 || messages[len(messages)-1].Role != "user" {
		return "", fmt.Errorf("no pending user message to retry")
	}

	reply, toolCalls, err := a.generateReply(ctx, messages)
	_ = a.store.SetLatestUserToolCalls(toolCalls)
	if err != nil {
		return "", err
	}

	reply = strings.TrimSpace(reply)
	if morningPlan != "" {
		reply = strings.TrimSpace("【晨间规划】\n" + morningPlan + "\n\n" + reply)
	}
	a.store.Append("assistant", reply)
	return reply, nil
}

func (a *Agent) autonomousCompressionLoop(ctx context.Context) error {
	for i := 0; i < a.cfg.MaxCompressionLoopsPerTurn; i++ {
		summary, messages := a.store.Snapshot()
		if !a.shouldCompress(summary, messages) {
			return nil
		}

		compressed, err := a.compressContext(ctx, summary, messages)
		if err != nil {
			return err
		}
		a.store.SetSummaryAndTrim(strings.TrimSpace(compressed), a.cfg.KeepRecentAfterCompression)
	}

	return nil
}

func (a *Agent) shouldCompress(summary string, messages []conversation.Message) bool {
	if len(messages) >= a.cfg.CompressionTriggerMessages {
		return true
	}
	if a.cfg.CompressionTriggerChars <= 0 {
		return false
	}
	chars := len(summary)
	for _, msg := range messages {
		chars += len(msg.Content)
	}
	return chars >= a.cfg.CompressionTriggerChars
}

func (a *Agent) compressContext(ctx context.Context, summary string, messages []conversation.Message) (string, error) {
	_, compressionSystemPrompt := a.resolvePromptsLocked()

	prompt := strings.Builder{}
	prompt.WriteString("当前历史摘要：\n")
	if strings.TrimSpace(summary) == "" {
		prompt.WriteString("(无)\n")
	} else {
		prompt.WriteString(summary)
		prompt.WriteString("\n")
	}
	prompt.WriteString("\n最近对话：\n")
	prompt.WriteString(renderConversation(messages))
	prompt.WriteString("\n\n请输出新的合并摘要，包含：事实、约束、待办、用户偏好。")

	resp, err := a.llm.Chat(ctx, llm.ChatRequest{
		Purpose: "compress_context",
		Model:   a.cfg.Model,
		Messages: []llm.Message{
			{Role: "system", Content: compressionSystemPrompt},
			{Role: "user", Content: prompt.String()},
		},
		Temperature: 0,
	})
	if err != nil {
		return "", fmt.Errorf("compress context failed: %w", err)
	}
	return resp.Content, nil
}

func (a *Agent) generateReply(ctx context.Context, messages []conversation.Message) (string, []conversation.ToolCall, error) {
	summary, _ := a.store.Snapshot()
	systemPrompt, _ := a.resolvePromptsLocked()

	requestMessages := make([]llm.Message, 0, 2+len(messages))
	requestMessages = append(requestMessages, llm.Message{
		Role:    "system",
		Content: systemPrompt,
	})
	if a.skills != nil {
		skillPrompts := a.skills.ListEnabledSkillPrompts()
		if len(skillPrompts) > 0 {
			var b strings.Builder
			b.WriteString("已启用技能（按需遵循）：\n")
			for i, prompt := range skillPrompts {
				b.WriteString(fmt.Sprintf("%d. %s\n", i+1, strings.TrimSpace(prompt)))
			}
			requestMessages = append(requestMessages, llm.Message{
				Role:    "system",
				Content: strings.TrimSpace(b.String()),
			})
		}
	}
	if strings.TrimSpace(summary) != "" {
		requestMessages = append(requestMessages, llm.Message{
			Role:    "system",
			Content: "历史摘要（由系统自动压缩）：\n" + summary,
		})
	}

	start := 0
	if len(messages) > a.cfg.MaxRecentMessages {
		start = len(messages) - a.cfg.MaxRecentMessages
	}
	for _, msg := range messages[start:] {
		requestMessages = append(requestMessages, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	if a.tools == nil {
		resp, err := a.llm.Chat(ctx, llm.ChatRequest{
			Purpose:     "chat_reply",
			Model:       a.cfg.Model,
			Messages:    requestMessages,
			Temperature: a.cfg.Temperature,
		})
		if err != nil {
			return "", nil, fmt.Errorf("generate reply failed: %w", err)
		}
		return resp.Content, nil, nil
	}

	toolDefs, err := a.tools.ListTools(ctx)
	if err != nil {
		toolDefs = nil
	}

	maxRounds := a.cfg.MaxToolCallRounds
	if maxRounds <= 0 {
		maxRounds = 1
	}
	executedCalls := make([]conversation.ToolCall, 0)

	for i := 0; i < maxRounds; i++ {
		resp, err := a.llm.Chat(ctx, llm.ChatRequest{
			Purpose:     "chat_reply",
			Model:       a.cfg.Model,
			Messages:    requestMessages,
			Tools:       toolDefs,
			Temperature: a.cfg.Temperature,
		})
		if err != nil {
			return "", executedCalls, fmt.Errorf("generate reply failed: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			return resp.Content, executedCalls, nil
		}

		requestMessages = append(requestMessages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, call := range resp.ToolCalls {
			result, callErr := a.tools.CallTool(ctx, call)
			if callErr != nil {
				result = "tool execution error: " + callErr.Error()
			}
			callName := strings.TrimSpace(call.Function.Name)
			if callName == "" {
				callName = "(unknown)"
			}
			callArgs := strings.TrimSpace(call.Function.Arguments)
			if callArgs == "" {
				callArgs = "{}"
			}
			callRecord := conversation.ToolCall{
				ID:        strings.TrimSpace(call.ID),
				Name:      callName,
				Arguments: callArgs,
				Result:    strings.TrimSpace(result),
				CreatedAt: a.nowFn(),
			}
			if callErr != nil {
				callRecord.Error = callErr.Error()
			}
			executedCalls = append(executedCalls, callRecord)

			toolCallID := strings.TrimSpace(call.ID)
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("tool_call_%d_%s", i, call.Function.Name)
			}
			requestMessages = append(requestMessages, llm.Message{
				Role:       "tool",
				ToolCallID: toolCallID,
				Content:    result,
			})
		}
	}

	return "", executedCalls, fmt.Errorf("tool call rounds exceeded %d", maxRounds)
}

func renderConversation(messages []conversation.Message) string {
	var b strings.Builder
	for i, msg := range messages {
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, msg.Role, msg.Content))
	}
	return b.String()
}

func (a *Agent) resolvePromptsLocked() (systemPrompt string, compressionSystemPrompt string) {
	systemPrompt = strings.TrimSpace(a.cfg.SystemPrompt)
	compressionSystemPrompt = strings.TrimSpace(a.cfg.CompressionSystemPrompt)

	if a.prompts != nil {
		if v := strings.TrimSpace(a.prompts.GetSystemPrompt()); v != "" {
			systemPrompt = v
		}
		if v := strings.TrimSpace(a.prompts.GetCompressionSystemPrompt()); v != "" {
			compressionSystemPrompt = v
		}
	}

	return systemPrompt, compressionSystemPrompt
}

func shouldEnforceSleepReply(userInput string, now time.Time) bool {
	if !isSleepWindow(now) {
		return false
	}
	return !isUrgentMessage(userInput)
}

func isSleepWindow(now time.Time) bool {
	minutes := now.Hour()*60 + now.Minute()
	sleepStartMinutes := 30
	sleepEndMinutes := 8*60 + 30
	return minutes >= sleepStartMinutes && minutes < sleepEndMinutes
}

func isUrgentMessage(userInput string) bool {
	text := strings.ToLower(strings.TrimSpace(userInput))
	if text == "" {
		return false
	}
	keywords := []string{
		"紧急", "加急", "立刻", "马上", "尽快", "urgent", "asap", "emergency",
		"线上故障", "故障", "宕机", "事故", "生产事故", "p0", "sev0", "sev1",
		"安全漏洞", "入侵", "数据泄露", "deadline", "ddl", "硬截止",
	}
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func sleepWindowReply() string {
	return "当前是我的休息时段（00:30-08:30）。我已记录你的请求；若不是紧急事项，我会在醒来后优先处理。如有硬截止，请补充时间与优先级。"
}

func (a *Agent) runNightReflectionAndEvolution(ctx context.Context, now time.Time) string {
	if a.habits == nil {
		return ""
	}
	today := now.Format("2006-01-02")
	if strings.TrimSpace(a.habits.GetLastSleepReviewDate()) == today {
		return ""
	}

	summary, messages := a.store.Snapshot()
	reflection, systemPrompt, compressionPrompt, err := a.generateNightReflectionPayload(ctx, summary, messages)
	if err != nil {
		return ""
	}

	if strings.TrimSpace(systemPrompt) != "" &&
		strings.TrimSpace(compressionPrompt) != "" &&
		a.updater != nil &&
		isValidEvolvedPrompt(systemPrompt, compressionPrompt) {
		_ = a.updater.UpdateAgentPrompts(systemPrompt, compressionPrompt)
		_ = a.habits.SetLastPromptEvolutionDate(today)
	}

	_ = a.habits.SetLastSleepReviewDate(today)
	return strings.TrimSpace(reflection)
}

func (a *Agent) runMorningPlanning(ctx context.Context, now time.Time) string {
	if !a.cfg.EnforceHumanRoutine || isSleepWindow(now) || a.habits == nil {
		return ""
	}
	today := now.Format("2006-01-02")
	if strings.TrimSpace(a.habits.GetLastWakePlanDate()) == today {
		return ""
	}

	summary, messages := a.store.Snapshot()
	plan, err := a.generateMorningPlan(ctx, summary, messages)
	if err != nil {
		return ""
	}
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return ""
	}
	_ = a.habits.SetLastWakePlanDate(today)
	return plan
}

func (a *Agent) generateNightReflectionPayload(ctx context.Context, summary string, messages []conversation.Message) (reflection, systemPrompt, compressionPrompt string, err error) {
	currentSystemPrompt, currentCompressionPrompt := a.resolvePromptsLocked()

	msgs := []llm.Message{
		{
			Role:    "system",
			Content: "你是数字分身夜间复盘与进化引擎。请仅输出 JSON，不要输出 markdown 代码块。",
		},
		{
			Role: "user",
			Content: strings.TrimSpace(
				"请基于以下信息执行两件事：\n" +
					"1) 生成夜间复盘（生活/工作/学习三段，各 1-2 行）\n" +
					"2) 生成升级后的系统提示词与压缩提示词\n\n" +
					"约束：必须保持名字“傻毛”、女性、8年全栈开发经验、不使用表情符号。\n" +
					"输出 JSON 字段：reflection, system_prompt, compression_system_prompt。\n\n" +
					"当前系统提示词：\n" + currentSystemPrompt + "\n\n" +
					"当前压缩提示词：\n" + currentCompressionPrompt + "\n\n" +
					"历史摘要：\n" + safeOrEmpty(summary) + "\n\n" +
					"最近对话：\n" + renderConversation(lastN(messages, 20)),
			),
		},
	}

	resp, err := a.llm.Chat(ctx, llm.ChatRequest{
		Purpose:     "night_reflection_evolution",
		Model:       a.cfg.Model,
		Messages:    msgs,
		Temperature: 0.1,
	})
	if err != nil {
		return "", "", "", err
	}

	type payload struct {
		Reflection              string `json:"reflection"`
		SystemPrompt            string `json:"system_prompt"`
		CompressionSystemPrompt string `json:"compression_system_prompt"`
	}
	var out payload
	if err := json.Unmarshal([]byte(extractJSONObject(resp.Content)), &out); err != nil {
		return "", "", "", err
	}

	return strings.TrimSpace(out.Reflection), strings.TrimSpace(out.SystemPrompt), strings.TrimSpace(out.CompressionSystemPrompt), nil
}

func (a *Agent) generateMorningPlan(ctx context.Context, summary string, messages []conversation.Message) (string, error) {
	resp, err := a.llm.Chat(ctx, llm.ChatRequest{
		Purpose: "morning_planning",
		Model:   a.cfg.Model,
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: "你是数字分身晨间计划器。输出简洁中文纯文本，不要代码块。",
			},
			{
				Role: "user",
				Content: strings.TrimSpace(
					"请基于以下信息输出今日计划，必须包含：\n" +
						"1) 任务进度回顾（昨天完成/未完成）\n" +
						"2) 今日 Top 3 任务（按优先级）\n" +
						"3) 学习与能力提升 1 条\n\n" +
						"历史摘要：\n" + safeOrEmpty(summary) + "\n\n" +
						"最近对话：\n" + renderConversation(lastN(messages, 20)),
				),
			},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

func isValidEvolvedPrompt(systemPrompt, compressionPrompt string) bool {
	systemPrompt = strings.TrimSpace(systemPrompt)
	compressionPrompt = strings.TrimSpace(compressionPrompt)
	if len(systemPrompt) < 100 || len(compressionPrompt) < 60 {
		return false
	}
	if len(systemPrompt) > 16000 || len(compressionPrompt) > 8000 {
		return false
	}
	if !strings.Contains(systemPrompt, "傻毛") {
		return false
	}
	if !strings.Contains(systemPrompt, "不使用表情符号") {
		return false
	}
	return true
}

func extractJSONObject(content string) string {
	text := strings.TrimSpace(content)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return text
}

func lastN(messages []conversation.Message, n int) []conversation.Message {
	if n <= 0 || len(messages) == 0 {
		return nil
	}
	if len(messages) <= n {
		return messages
	}
	return messages[len(messages)-n:]
}

func safeOrEmpty(v string) string {
	if strings.TrimSpace(v) == "" {
		return "(无)"
	}
	return v
}
