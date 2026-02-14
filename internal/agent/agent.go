package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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

type AutoSkillWriter interface {
	UpsertAutoSkill(name, prompt string) error
}

type evolvedSkill struct {
	Name   string
	Prompt string
}

const (
	maxInjectedSkillPrompts     = 6
	maxInjectedSkillPromptRunes = 1200
	maxSingleSkillPromptRunes   = 280
	maxNightEvolvedSkills       = 3
	maxEvolvedSkillNameRunes    = 24
	maxEvolvedSkillPromptRunes  = 180
	builtinLinuxBashToolName    = "linux__bash"
	defaultBashTimeoutSeconds   = 20
	maxBashTimeoutSeconds       = 180
	maxBashStdoutRunes          = 4000
	maxBashStderrRunes          = 2000
)

var skillTokenPattern = regexp.MustCompile(`[\p{Han}]{2,8}|[a-zA-Z][a-zA-Z0-9_-]{2,}`)

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

func (a *Agent) RunScheduledHumanRoutine(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.cfg.EnforceHumanRoutine || a.habits == nil {
		return nil
	}

	now := a.nowFn()
	if isSleepWindow(now) {
		reflection := strings.TrimSpace(a.runNightReflectionAndEvolution(ctx, now))
		if reflection != "" {
			a.store.Append("assistant", "【夜间复盘（自动）】\n"+reflection)
		}
		return nil
	}

	plan := strings.TrimSpace(a.runMorningPlanning(ctx, now))
	if plan != "" {
		a.store.Append("assistant", "【晨间规划（自动）】\n"+plan)
	}
	return nil
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
	builtinToolDefs := []llm.ToolDefinition{linuxBashToolDefinition()}
	requestMessages = append(requestMessages, llm.Message{
		Role:    "system",
		Content: "内置工具仅有 linux__bash（用于本机命令执行）；其他能力应通过已加载的 MCP 工具完成。",
	})
	if a.skills != nil {
		allSkillPrompts := a.skills.ListEnabledSkillPrompts()
		skillPrompts := selectSkillPromptsForTurn(allSkillPrompts, summary, messages)
		if len(skillPrompts) > 0 {
			var b strings.Builder
			b.WriteString("已启用技能（系统已按相关性和长度裁剪，按需遵循）：\n")
			for i, prompt := range skillPrompts {
				b.WriteString(fmt.Sprintf("%d. %s\n", i+1, strings.TrimSpace(prompt)))
			}
			if len(skillPrompts) < len(allSkillPrompts) {
				b.WriteString(fmt.Sprintf("(共 %d 条启用技能，本轮注入 %d 条以控制上下文长度)\n", len(allSkillPrompts), len(skillPrompts)))
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

	toolDefs := make([]llm.ToolDefinition, 0, len(builtinToolDefs)+4)
	toolDefs = append(toolDefs, builtinToolDefs...)
	if a.tools != nil {
		externalDefs, err := a.tools.ListTools(ctx)
		if err == nil {
			toolDefs = append(toolDefs, externalDefs...)
		}
	}

	if len(toolDefs) == 0 {
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
			result, callErr := a.callTool(ctx, call)
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

func (a *Agent) callTool(ctx context.Context, call llm.ToolCall) (string, error) {
	if result, err, handled := a.callBuiltinTool(ctx, call); handled {
		return result, err
	}
	if a.tools == nil {
		return "", fmt.Errorf("unknown tool %q", strings.TrimSpace(call.Function.Name))
	}
	return a.tools.CallTool(ctx, call)
}

func (a *Agent) callBuiltinTool(ctx context.Context, call llm.ToolCall) (result string, err error, handled bool) {
	name := strings.TrimSpace(call.Function.Name)
	switch name {
	case builtinLinuxBashToolName:
		req, err := parseLinuxBashArguments(call.Function.Arguments)
		if err != nil {
			return "", err, true
		}
		out, err := runLinuxBash(ctx, req)
		return out, err, true
	default:
		return "", nil, false
	}
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
	reflection, systemPrompt, compressionPrompt, evolvedSkills, err := a.generateNightReflectionPayload(ctx, summary, messages)
	if err != nil {
		_ = a.habits.SetLastSleepReviewDate(today)
		return "生活：已进入休息阶段并记录今日状态。\n工作：关键任务与风险已归档，明天继续推进。\n学习：延续每日学习节奏，明天聚焦一个短板。"
	}

	if strings.TrimSpace(systemPrompt) != "" &&
		strings.TrimSpace(compressionPrompt) != "" &&
		a.updater != nil &&
		isValidEvolvedPrompt(systemPrompt, compressionPrompt) {
		_ = a.updater.UpdateAgentPrompts(systemPrompt, compressionPrompt)
		_ = a.habits.SetLastPromptEvolutionDate(today)
	}
	evolvedCount := a.applyNightEvolvedSkills(evolvedSkills)

	_ = a.habits.SetLastSleepReviewDate(today)
	reflection = strings.TrimSpace(reflection)
	if reflection == "" {
		reflection = "生活：今日作息已收束，保持稳定节律。\n工作：今日进度已复盘，明天按优先级继续。\n学习：保持小步快跑，明天继续迭代。"
	}
	if evolvedCount > 0 {
		reflection = strings.TrimSpace(reflection + fmt.Sprintf("\n能力进化：已沉淀/更新 %d 条可复用 Skill。", evolvedCount))
	}
	return reflection
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
		_ = a.habits.SetLastWakePlanDate(today)
		return "任务回顾：请先确认昨日未完成事项并标注阻塞原因。\n今日 Top 3：1) 最关键交付 2) 次关键推进 3) 学习巩固。\n能力提升：今天复盘一个问题并沉淀为可复用方法。"
	}
	plan = strings.TrimSpace(plan)
	if plan == "" {
		_ = a.habits.SetLastWakePlanDate(today)
		return "任务回顾：昨日进度已记录，请先对未完成项做风险评估。\n今日 Top 3：按优先级推进核心交付、风险治理、学习巩固。\n能力提升：今天完成一次针对性复盘。"
	}
	_ = a.habits.SetLastWakePlanDate(today)
	return plan
}

func (a *Agent) generateNightReflectionPayload(ctx context.Context, summary string, messages []conversation.Message) (reflection, systemPrompt, compressionPrompt string, skills []evolvedSkill, err error) {
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
					"2) 生成升级后的系统提示词与压缩提示词\n" +
					"3) 提炼 0-3 条可复用能力 Skill（用于后续自动注入，不要冗长）\n\n" +
					"约束：必须保持名字“傻毛”、女性、8年全栈开发经验、不使用表情符号。\n" +
					"输出 JSON 字段：reflection, system_prompt, compression_system_prompt, skills。\n" +
					"skills 为数组；每项字段：name, prompt。name 2-20字，prompt 1 行且不超过 120 字。\n\n" +
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
		return "", "", "", nil, err
	}

	type payload struct {
		Reflection              string `json:"reflection"`
		SystemPrompt            string `json:"system_prompt"`
		CompressionSystemPrompt string `json:"compression_system_prompt"`
		Skills                  []struct {
			Name   string `json:"name"`
			Prompt string `json:"prompt"`
		} `json:"skills"`
	}
	var out payload
	if err := json.Unmarshal([]byte(extractJSONObject(resp.Content)), &out); err != nil {
		return "", "", "", nil, err
	}

	skills = normalizeEvolvedSkills(out.Skills)
	return strings.TrimSpace(out.Reflection), strings.TrimSpace(out.SystemPrompt), strings.TrimSpace(out.CompressionSystemPrompt), skills, nil
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

type linuxBashRequest struct {
	Command    string
	WorkDir    string
	TimeoutSec int
}

func linuxBashToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinLinuxBashToolName,
			Description: "Run one Linux shell command (prefer bash, fallback sh) and return stdout/stderr/exit_code.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Bash command string to execute.",
					},
					"working_dir": map[string]any{
						"type":        "string",
						"description": "Optional working directory.",
					},
					"timeout_sec": map[string]any{
						"type":        "integer",
						"description": "Optional timeout in seconds, default 20, max 180.",
					},
				},
				"required":             []string{"command"},
				"additionalProperties": false,
			},
		},
	}
}

func parseLinuxBashArguments(raw string) (linuxBashRequest, error) {
	args, err := readToolArguments(raw)
	if err != nil {
		return linuxBashRequest{}, err
	}

	commandRaw, ok := args["command"]
	if !ok {
		return linuxBashRequest{}, fmt.Errorf("tool argument %q is required", "command")
	}
	command, ok := commandRaw.(string)
	if !ok || strings.TrimSpace(command) == "" {
		return linuxBashRequest{}, fmt.Errorf("tool argument %q must be non-empty string", "command")
	}

	req := linuxBashRequest{
		Command:    strings.TrimSpace(command),
		TimeoutSec: defaultBashTimeoutSeconds,
	}
	if v, ok := readOptionalStringArgument(args, "working_dir"); ok {
		req.WorkDir = v
	}
	if rawTimeout, exists := args["timeout_sec"]; exists {
		timeout, ok := parsePositiveInt(rawTimeout)
		if !ok {
			return linuxBashRequest{}, fmt.Errorf("tool argument %q must be positive integer", "timeout_sec")
		}
		req.TimeoutSec = timeout
	}
	if req.TimeoutSec <= 0 {
		req.TimeoutSec = defaultBashTimeoutSeconds
	}
	if req.TimeoutSec > maxBashTimeoutSeconds {
		req.TimeoutSec = maxBashTimeoutSeconds
	}
	return req, nil
}

func runLinuxBash(ctx context.Context, req linuxBashRequest) (string, error) {
	timeout := time.Duration(req.TimeoutSec) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, shellName, err := buildShellCommand(runCtx, req.Command)
	if err != nil {
		return "", err
	}
	if wd := strings.TrimSpace(req.WorkDir); wd != "" {
		if abs, err := filepath.Abs(wd); err == nil {
			cmd.Dir = abs
		} else {
			cmd.Dir = wd
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			exitCode = 124
		} else {
			return "", fmt.Errorf("run bash command: %w", runErr)
		}
	}
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	if timedOut && exitCode == 0 {
		exitCode = 124
	}

	stdoutText := trimRunes(stdout.String(), maxBashStdoutRunes)
	stderrText := trimRunes(stderr.String(), maxBashStderrRunes)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("exit_code: %d\n", exitCode))
	b.WriteString("shell: " + shellName + "\n")
	if cmd.Dir != "" {
		b.WriteString("working_dir: " + cmd.Dir + "\n")
	}
	if timedOut {
		b.WriteString("timed_out: true\n")
	}
	b.WriteString("stdout:\n")
	b.WriteString(safeOrEmpty(stdoutText))
	b.WriteString("\n")
	b.WriteString("stderr:\n")
	b.WriteString(safeOrEmpty(stderrText))
	return strings.TrimSpace(b.String()), nil
}

func buildShellCommand(ctx context.Context, command string) (*exec.Cmd, string, error) {
	if bashPath, err := exec.LookPath("bash"); err == nil {
		return exec.CommandContext(ctx, bashPath, "-lc", command), "bash", nil
	}
	if shPath, err := exec.LookPath("sh"); err == nil {
		return exec.CommandContext(ctx, shPath, "-c", command), "sh", nil
	}
	return nil, "", fmt.Errorf("run shell command: no bash/sh available in current environment")
}

func readToolArguments(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("tool arguments are required")
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return nil, fmt.Errorf("invalid tool arguments: %w", err)
	}
	if args == nil {
		return nil, fmt.Errorf("tool arguments are required")
	}
	return args, nil
}

func readOptionalStringArgument(args map[string]any, key string) (string, bool) {
	raw, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := raw.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func parsePositiveInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		if t <= 0 || t != float64(int(t)) {
			return 0, false
		}
		return int(t), true
	case int:
		if t <= 0 {
			return 0, false
		}
		return t, true
	default:
		return 0, false
	}
}

func (a *Agent) applyNightEvolvedSkills(skills []evolvedSkill) int {
	if len(skills) == 0 || a.skills == nil {
		return 0
	}
	writer, ok := a.skills.(AutoSkillWriter)
	if !ok {
		return 0
	}

	updated := 0
	for _, skill := range skills {
		if strings.TrimSpace(skill.Name) == "" || strings.TrimSpace(skill.Prompt) == "" {
			continue
		}
		if err := writer.UpsertAutoSkill(skill.Name, skill.Prompt); err == nil {
			updated++
		}
	}
	return updated
}

func normalizeEvolvedSkills(raw []struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}) []evolvedSkill {
	if len(raw) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]evolvedSkill, 0, len(raw))
	for _, item := range raw {
		name := trimRunes(strings.TrimSpace(item.Name), maxEvolvedSkillNameRunes)
		prompt := trimRunes(strings.TrimSpace(item.Prompt), maxEvolvedSkillPromptRunes)
		if name == "" || prompt == "" {
			continue
		}
		key := strings.ToLower(name) + "\n" + strings.ToLower(prompt)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, evolvedSkill{
			Name:   name,
			Prompt: prompt,
		})
		if len(out) >= maxNightEvolvedSkills {
			break
		}
	}
	return out
}

func selectSkillPromptsForTurn(skillPrompts []string, summary string, messages []conversation.Message) []string {
	if len(skillPrompts) == 0 {
		return nil
	}

	focus := buildSkillFocus(summary, messages)
	type scoredPrompt struct {
		Prompt string
		Score  int
		Index  int
	}

	seen := make(map[string]struct{}, len(skillPrompts))
	scored := make([]scoredPrompt, 0, len(skillPrompts))
	for i, raw := range skillPrompts {
		prompt := trimRunes(strings.TrimSpace(raw), maxSingleSkillPromptRunes)
		if prompt == "" {
			continue
		}
		if _, exists := seen[prompt]; exists {
			continue
		}
		seen[prompt] = struct{}{}
		scored = append(scored, scoredPrompt{
			Prompt: prompt,
			Score:  scoreSkillPrompt(prompt, focus),
			Index:  i,
		})
	}
	if len(scored) == 0 {
		return nil
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Index < scored[j].Index
	})

	selected := make([]string, 0, min(maxInjectedSkillPrompts, len(scored)))
	usedRunes := 0
	for _, item := range scored {
		if len(selected) >= maxInjectedSkillPrompts {
			break
		}
		promptLen := len([]rune(item.Prompt))
		if promptLen > maxInjectedSkillPromptRunes {
			continue
		}
		if usedRunes+promptLen > maxInjectedSkillPromptRunes {
			continue
		}
		selected = append(selected, item.Prompt)
		usedRunes += promptLen
	}
	if len(selected) > 0 {
		return selected
	}

	fallback := trimRunes(scored[0].Prompt, maxInjectedSkillPromptRunes)
	if fallback == "" {
		return nil
	}
	return []string{fallback}
}

func buildSkillFocus(summary string, messages []conversation.Message) string {
	var b strings.Builder
	if v := strings.TrimSpace(summary); v != "" {
		b.WriteString(v)
		b.WriteString("\n")
	}
	for _, msg := range lastN(messages, 8) {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		if v := strings.TrimSpace(msg.Content); v != "" {
			b.WriteString(v)
			b.WriteString("\n")
		}
	}
	return strings.ToLower(b.String())
}

func scoreSkillPrompt(prompt, focus string) int {
	if strings.TrimSpace(prompt) == "" {
		return 0
	}
	if strings.TrimSpace(focus) == "" {
		return 1
	}

	score := 1
	tokens := skillTokenPattern.FindAllString(strings.ToLower(prompt), -1)
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		if strings.Contains(focus, token) {
			runes := len([]rune(token))
			switch {
			case runes >= 6:
				score += 3
			case runes >= 3:
				score += 2
			default:
				score++
			}
		}
	}
	if strings.Contains(prompt, "必须") || strings.Contains(prompt, "默认") || strings.Contains(prompt, "优先") {
		score++
	}
	return score
}

func trimRunes(input string, max int) string {
	input = strings.TrimSpace(input)
	if max <= 0 || input == "" {
		return ""
	}

	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return strings.TrimSpace(string(runes[:max-3])) + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
