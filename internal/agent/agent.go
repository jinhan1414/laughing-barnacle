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
	"strconv"
	"strings"
	"sync"
	"time"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"laughing-barnacle/internal/routine"
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
	ListEnabledSkillIndex() []string
	ReadEnabledSkillPrompt(skillID string) (string, bool)
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
	maxSingleSkillPromptRunes   = 600
	minInjectedSkillScore       = 3
	maxSkillFocusUserMessages   = 4
	maxNightEvolvedSkills       = 3
	maxEvolvedSkillNameRunes    = 24
	maxEvolvedSkillPromptRunes  = 180
	maxScheduledRecentMessages  = 20
	builtinLinuxBashToolName    = "linux__bash"
	defaultBashTimeoutSeconds   = 20
	maxBashTimeoutSeconds       = 180
	maxBashStdoutRunes          = 4000
	maxBashStderrRunes          = 2000
	maxGreetingRecentMessages   = 8
	maxGreetingTaskStatuses     = 5
)

var (
	skillTokenPattern         = regexp.MustCompile(`[\p{Han}]{2,8}|[a-zA-Z][a-zA-Z0-9_-]{2,}`)
	comparableStripPattern    = regexp.MustCompile(`[[:space:][:punct:]，。！？；：、“”‘’（）【】《》·]+`)
	numberedListPrefixPattern = regexp.MustCompile(`^\d+[.)、]\s*`)
)

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

type ChatGreetingInput struct {
	Now                 time.Time
	IsFirstToday        bool
	LastGreetingAt      time.Time
	LastGreetingContent string
	RecentTaskStatuses  []string
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
	now := a.nowFn()
	if isSleepWindow(now) {
		return a.RunScheduledTask(ctx, routine.ActionNightReflectionEvolution)
	}
	return a.RunScheduledTask(ctx, routine.ActionMorningPlanning)
}

func (a *Agent) RunScheduledTask(ctx context.Context, action string) error {
	action = routine.NormalizeAction(strings.TrimSpace(action))

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.cfg.EnforceHumanRoutine {
		return nil
	}

	now := a.nowFn()
	skillID, ok := routine.SkillIDFromAction(action)
	if !ok {
		err := fmt.Errorf("unknown scheduled action: %s", action)
		a.appendScheduledTaskFailureLocked(action, err)
		return err
	}

	var (
		title   string
		content string
		err     error
	)
	switch {
	case routine.IsNightReflectionSkillID(skillID):
		title = "夜间复盘（自动）"
		content, err = a.runNightReflectionAndEvolution(ctx, now, skillID)
	case routine.IsMorningPlanningSkillID(skillID):
		title = "晨间规划（自动）"
		content, err = a.runMorningPlanning(ctx, now, skillID)
	default:
		title, content, err = a.runGenericScheduledSkill(ctx, now, skillID)
	}
	if err != nil {
		a.appendScheduledTaskFailureLocked(action, err)
		return err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if strings.TrimSpace(title) == "" {
		a.store.Append("assistant", content)
		return nil
	}
	a.store.Append("assistant", "【"+strings.TrimSpace(title)+"】\n"+content)
	return nil
}

func (a *Agent) appendScheduledTaskFailureLocked(action string, runErr error) {
	if a == nil || a.store == nil || runErr == nil {
		return
	}
	action = strings.TrimSpace(action)
	if action == "" {
		action = "(unknown)"
	}
	errText := strings.TrimSpace(runErr.Error())
	if errText == "" {
		errText = "unknown error"
	}
	a.store.Append("assistant", "【定时任务执行失败】\n任务："+action+"\n状态：失败\n原因："+trimRunes(errText, 180))
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
	a.store.Append("assistant", reply)
	return reply, nil
}

func (a *Agent) GenerateChatGreeting(ctx context.Context, input ChatGreetingInput) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := input.Now
	if now.IsZero() {
		now = a.nowFn()
	}

	summary, messages := a.store.Snapshot()
	recentConversation := strings.TrimSpace(renderConversation(lastN(messages, maxGreetingRecentMessages)))
	if recentConversation == "" {
		recentConversation = "(无)"
	}

	taskLines := make([]string, 0, min(len(input.RecentTaskStatuses), maxGreetingTaskStatuses))
	for _, raw := range input.RecentTaskStatuses {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		taskLines = append(taskLines, trimRunes(line, 180))
		if len(taskLines) >= maxGreetingTaskStatuses {
			break
		}
	}
	recentTaskStatuses := "(无)"
	if len(taskLines) > 0 {
		recentTaskStatuses = "- " + strings.Join(taskLines, "\n- ")
	}

	lastGreetingAt := "(无)"
	if !input.LastGreetingAt.IsZero() {
		lastGreetingAt = input.LastGreetingAt.Local().Format("2006-01-02 15:04:05")
	}
	turnType := "当日再次进入聊天页（系统已确认当前没有任务执行中）"
	if input.IsFirstToday {
		turnType = "当日首次进入聊天页"
	}

	resp, err := a.llm.Chat(ctx, llm.ChatRequest{
		Purpose: "chat_greeting",
		Model:   a.cfg.Model,
		Messages: []llm.Message{
			{
				Role: "system",
				Content: "你是产品内的数字分身。你现在只负责生成“用户打开聊天页时的主动问候”。" +
					"必须输出简短中文 1-2 句，不超过 60 字，不使用 markdown，不要虚构已完成或正在执行的任务。",
			},
			{
				Role: "user",
				Content: strings.TrimSpace(
					"请基于以下上下文生成问候：\n" +
						"- 当前时间：" + now.Format("2006-01-02 15:04:05") + "\n" +
						"- 进入类型：" + turnType + "\n" +
						"- 当前是否有任务执行中：否\n" +
						"- 上次主动问候时间：" + lastGreetingAt + "\n" +
						"- 上次主动问候内容（避免重复）：" + safeOrEmpty(trimRunes(input.LastGreetingContent, 120)) + "\n" +
						"- 历史摘要：" + safeOrEmpty(summary) + "\n" +
						"- 最近任务状态：\n" + recentTaskStatuses + "\n" +
						"- 最近对话：\n" + recentConversation + "\n\n" +
						"请直接输出问候正文。",
				),
			},
		},
		Temperature: 0.3,
	})
	if err != nil {
		return "", fmt.Errorf("generate chat greeting failed: %w", err)
	}
	return strings.TrimSpace(resp.Content), nil
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
		compressed = strings.TrimSpace(compressed)
		systemPrompt, _ := a.resolvePromptsLocked()
		compressed = pruneSummaryOverlap(compressed, systemPrompt)
		a.store.SetSummaryAndTrim(compressed, a.cfg.KeepRecentAfterCompression)
		if compressed != "" {
			a.store.AppendEvent("context_compression", compressed)
		}
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
	systemPrompt, compressionSystemPrompt := a.resolvePromptsLocked()
	summary = pruneSummaryOverlap(summary, systemPrompt, compressionSystemPrompt)

	prompt := strings.Builder{}
	prompt.WriteString("当前历史摘要（仅作合并参考，不要原样复述）：\n")
	if strings.TrimSpace(summary) == "" {
		prompt.WriteString("(无)\n")
	} else {
		prompt.WriteString(summary)
		prompt.WriteString("\n")
	}
	prompt.WriteString("\n最近对话（仅 user/assistant）：\n")
	prompt.WriteString(renderConversationForCompression(messages))
	prompt.WriteString("\n\n请仅基于以上对话内容与已有摘要合并，输出新的摘要（事实、约束、待办、用户偏好），不要写入任何 system 提示词文本。")

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
	toolIntro := "内置工具仅有 linux__bash（用于本机命令执行）；其他能力应通过已加载的 MCP 工具完成。"
	skillIndexPrompt := ""
	if a.skills != nil {
		allSkillIndex := a.skills.ListEnabledSkillIndex()
		if len(allSkillIndex) > 0 {
			selectedSkillIDs := selectSkillIDsForTurn(allSkillIndex, messages)
			var b strings.Builder
			b.WriteString(fmt.Sprintf("已启用技能索引（渐进式披露）：共 %d 条。\n", len(allSkillIndex)))
			b.WriteString("如需技能详情，仅在必要时通过 linux__bash 执行：curl -s \"http://127.0.0.1:8080/api/skills/read?id=<skill_id>\"。\n")
			if len(selectedSkillIDs) > 0 {
				b.WriteString("本轮高相关候选 skill_id：")
				b.WriteString(strings.Join(selectedSkillIDs, ", "))
				b.WriteString("\n")
			}

			injected := 0
			usedRunes := 0
			for _, raw := range allSkillIndex {
				if injected >= maxInjectedSkillPrompts {
					break
				}
				line := trimRunes(strings.TrimSpace(raw), maxSingleSkillPromptRunes)
				if line == "" {
					continue
				}
				lineLen := len([]rune(line))
				if usedRunes+lineLen > maxInjectedSkillPromptRunes {
					break
				}
				b.WriteString(fmt.Sprintf("%d. %s\n", injected+1, line))
				injected++
				usedRunes += lineLen
			}
			if injected < len(allSkillIndex) {
				b.WriteString(fmt.Sprintf("(索引共 %d 条，本轮展示 %d 条以控制上下文长度)\n", len(allSkillIndex), injected))
			}
			skillIndexPrompt = strings.TrimSpace(b.String())
			requestMessages = append(requestMessages, llm.Message{
				Role:    "system",
				Content: skillIndexPrompt,
			})
		}
	}
	requestMessages = append(requestMessages, llm.Message{
		Role:    "system",
		Content: toolIntro,
	})
	summary = pruneSummaryOverlap(summary, systemPrompt, toolIntro, skillIndexPrompt)
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

func renderConversationForCompression(messages []conversation.Message) string {
	var b strings.Builder
	idx := 1
	for _, msg := range messages {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", idx, role, content))
		idx++
	}
	if idx == 1 {
		return "(无)"
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

func isSleepWindow(now time.Time) bool {
	minutes := now.Hour()*60 + now.Minute()
	sleepStartMinutes := 30
	sleepEndMinutes := 8*60 + 30
	return minutes >= sleepStartMinutes && minutes < sleepEndMinutes
}

func (a *Agent) runNightReflectionAndEvolution(ctx context.Context, now time.Time, skillID string) (string, error) {
	if a.habits == nil {
		return "", nil
	}
	today := now.Format("2006-01-02")
	if strings.TrimSpace(a.habits.GetLastSleepReviewDate()) == today {
		return "", nil
	}

	_, skillPrompt, ok := a.readScheduledSkill(skillID)
	if !ok {
		return "", fmt.Errorf("scheduled skill %q not found or not enabled", skillID)
	}

	summary, messages := a.store.Snapshot()
	reflection, systemPrompt, compressionPrompt, evolvedSkills, err := a.generateNightReflectionPayload(ctx, skillPrompt, summary, messages)
	if err != nil {
		_ = a.habits.SetLastSleepReviewDate(today)
		return "生活：已进入休息阶段并记录今日状态。\n工作：关键任务与风险已归档，明天继续推进。\n学习：延续每日学习节奏，明天聚焦一个短板。", nil
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
	return reflection, nil
}

func (a *Agent) runMorningPlanning(ctx context.Context, now time.Time, skillID string) (string, error) {
	if isSleepWindow(now) || a.habits == nil {
		return "", nil
	}
	today := now.Format("2006-01-02")
	if strings.TrimSpace(a.habits.GetLastWakePlanDate()) == today {
		return "", nil
	}

	_, skillPrompt, ok := a.readScheduledSkill(skillID)
	if !ok {
		return "", fmt.Errorf("scheduled skill %q not found or not enabled", skillID)
	}

	summary, messages := a.store.Snapshot()
	plan, err := a.generateMorningPlan(ctx, skillPrompt, summary, messages)
	if err != nil {
		_ = a.habits.SetLastWakePlanDate(today)
		return "任务回顾：请先确认昨日未完成事项并标注阻塞原因。\n今日 Top 3：1) 最关键交付 2) 次关键推进 3) 学习巩固。\n能力提升：今天复盘一个问题并沉淀为可复用方法。", nil
	}
	plan = strings.TrimSpace(plan)
	if plan == "" {
		_ = a.habits.SetLastWakePlanDate(today)
		return "任务回顾：昨日进度已记录，请先对未完成项做风险评估。\n今日 Top 3：按优先级推进核心交付、风险治理、学习巩固。\n能力提升：今天完成一次针对性复盘。", nil
	}
	_ = a.habits.SetLastWakePlanDate(today)
	return plan, nil
}

func (a *Agent) runGenericScheduledSkill(ctx context.Context, _ time.Time, skillID string) (title string, content string, err error) {
	skillName, skillPrompt, ok := a.readScheduledSkill(skillID)
	if !ok {
		return "", "", fmt.Errorf("scheduled skill %q not found or not enabled", skillID)
	}
	summary, messages := a.store.Snapshot()
	content, err = a.generateGenericScheduledOutput(ctx, skillID, skillPrompt, summary, messages)
	if err != nil {
		return "", "", err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return "", "", nil
	}
	if strings.TrimSpace(skillName) == "" {
		skillName = skillID
	}
	return "定时任务（自动）" + skillName, content, nil
}

func (a *Agent) generateNightReflectionPayload(ctx context.Context, skillPrompt, summary string, messages []conversation.Message) (reflection, systemPrompt, compressionPrompt string, skills []evolvedSkill, err error) {
	currentSystemPrompt, currentCompressionPrompt := a.resolvePromptsLocked()

	msgs := []llm.Message{
		{
			Role:    "system",
			Content: "你是数字分身定时任务执行器。必须严格遵循技能说明，若技能要求 JSON 则仅输出 JSON。",
		},
		{
			Role: "user",
			Content: strings.TrimSpace(
				"技能说明（来自 SKILL.md 正文）：\n" + strings.TrimSpace(skillPrompt) + "\n\n" +
					"当前系统提示词：\n" + currentSystemPrompt + "\n\n" +
					"当前压缩提示词：\n" + currentCompressionPrompt + "\n\n" +
					"历史摘要：\n" + safeOrEmpty(summary) + "\n\n" +
					"最近对话：\n" + renderConversation(lastN(messages, maxScheduledRecentMessages)) + "\n\n" +
					"请仅按技能说明完成任务并输出。",
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

func (a *Agent) generateMorningPlan(ctx context.Context, skillPrompt, summary string, messages []conversation.Message) (string, error) {
	currentSystemPrompt, currentCompressionPrompt := a.resolvePromptsLocked()
	resp, err := a.llm.Chat(ctx, llm.ChatRequest{
		Purpose: "morning_planning",
		Model:   a.cfg.Model,
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: "你是数字分身定时任务执行器。必须严格遵循技能说明，若技能要求 JSON 则仅输出 JSON。",
			},
			{
				Role: "user",
				Content: strings.TrimSpace(
					"技能说明（来自 SKILL.md 正文）：\n" + strings.TrimSpace(skillPrompt) + "\n\n" +
						"当前系统提示词：\n" + currentSystemPrompt + "\n\n" +
						"当前压缩提示词：\n" + currentCompressionPrompt + "\n\n" +
						"历史摘要：\n" + safeOrEmpty(summary) + "\n\n" +
						"最近对话：\n" + renderConversation(lastN(messages, maxScheduledRecentMessages)) + "\n\n" +
						"请仅按技能说明完成任务并输出。",
				),
			},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(resp.Content)
	if out == "" {
		return "", nil
	}

	type payload struct {
		Plan string `json:"plan"`
	}
	var parsed payload
	if err := json.Unmarshal([]byte(extractJSONObject(out)), &parsed); err == nil {
		if plan := strings.TrimSpace(parsed.Plan); plan != "" {
			return plan, nil
		}
	}
	return out, nil
}

func (a *Agent) generateGenericScheduledOutput(ctx context.Context, skillID, skillPrompt, summary string, messages []conversation.Message) (string, error) {
	currentSystemPrompt, currentCompressionPrompt := a.resolvePromptsLocked()
	resp, err := a.llm.Chat(ctx, llm.ChatRequest{
		Purpose: scheduledSkillPurpose(skillID),
		Model:   a.cfg.Model,
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: "你是数字分身定时任务执行器。必须严格遵循技能说明，若技能要求 JSON 则仅输出 JSON。",
			},
			{
				Role: "user",
				Content: strings.TrimSpace(
					"技能说明（来自 SKILL.md 正文）：\n" + strings.TrimSpace(skillPrompt) + "\n\n" +
						"当前系统提示词：\n" + currentSystemPrompt + "\n\n" +
						"当前压缩提示词：\n" + currentCompressionPrompt + "\n\n" +
						"历史摘要：\n" + safeOrEmpty(summary) + "\n\n" +
						"最近对话：\n" + renderConversation(lastN(messages, maxScheduledRecentMessages)) + "\n\n" +
						"请仅按技能说明完成任务并输出。",
				),
			},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}

	out := strings.TrimSpace(resp.Content)
	if out == "" {
		return "", nil
	}

	type payload struct {
		Content string `json:"content"`
		Result  string `json:"result"`
		Text    string `json:"text"`
	}
	var parsed payload
	if err := json.Unmarshal([]byte(extractJSONObject(out)), &parsed); err == nil {
		if v := strings.TrimSpace(parsed.Content); v != "" {
			return v, nil
		}
		if v := strings.TrimSpace(parsed.Result); v != "" {
			return v, nil
		}
		if v := strings.TrimSpace(parsed.Text); v != "" {
			return v, nil
		}
	}
	return out, nil
}

func (a *Agent) readScheduledSkill(skillID string) (name string, prompt string, ok bool) {
	if a.skills == nil {
		return "", "", false
	}
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return "", "", false
	}

	markdown, ok := a.skills.ReadEnabledSkillPrompt(skillID)
	if !ok {
		return "", "", false
	}
	name, prompt = parseSkillMarkdownForExecution(markdown)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", "", false
	}
	if strings.TrimSpace(name) == "" {
		name = skillID
	}
	return strings.TrimSpace(name), prompt, true
}

func parseSkillMarkdownForExecution(markdown string) (name string, prompt string) {
	text := strings.TrimSpace(strings.ReplaceAll(markdown, "\r\n", "\n"))
	if text == "" {
		return "", ""
	}
	if !strings.HasPrefix(text, "---\n") {
		return "", text
	}

	rest := strings.TrimPrefix(text, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", text
	}
	header := rest[:idx]
	body := strings.TrimSpace(rest[idx+5:])

	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			if unquoted, err := strconv.Unquote(value); err == nil {
				value = unquoted
			}
		}
		if key == "name" {
			name = value
		}
	}
	if strings.TrimSpace(body) == "" {
		body = text
	}
	return strings.TrimSpace(name), strings.TrimSpace(body)
}

func scheduledSkillPurpose(skillID string) string {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return "scheduled_skill"
	}
	var b strings.Builder
	for _, r := range skillID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		if r == '-' {
			b.WriteRune('_')
			continue
		}
	}
	suffix := strings.TrimSpace(b.String())
	if suffix == "" {
		return "scheduled_skill"
	}
	return "scheduled_skill_" + suffix
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

func selectSkillIDsForTurn(skillIndexLines []string, messages []conversation.Message) []string {
	if len(skillIndexLines) == 0 {
		return nil
	}

	focus := buildSkillFocus(messages)
	type scoredSkill struct {
		ID    string
		Score int
		Index int
	}

	seen := make(map[string]struct{}, len(skillIndexLines))
	scored := make([]scoredSkill, 0, len(skillIndexLines))
	for i, raw := range skillIndexLines {
		skillID, line := parseSkillIndexLine(raw)
		if skillID == "" || line == "" {
			continue
		}
		if _, exists := seen[skillID]; exists {
			continue
		}
		seen[skillID] = struct{}{}
		scored = append(scored, scoredSkill{
			ID:    skillID,
			Score: scoreSkillPrompt(line, focus),
			Index: i,
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
	for _, item := range scored {
		if len(selected) >= maxInjectedSkillPrompts {
			break
		}
		if item.Score < minInjectedSkillScore {
			break
		}
		selected = append(selected, item.ID)
	}
	return selected
}

func parseSkillIndexLine(raw string) (skillID string, line string) {
	line = trimRunes(strings.TrimSpace(raw), maxSingleSkillPromptRunes)
	if line == "" {
		return "", ""
	}
	for _, part := range strings.Split(line, "|") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "skill_id=") {
			skillID = strings.TrimSpace(strings.TrimPrefix(part, "skill_id="))
			break
		}
	}
	return skillID, line
}

func buildSkillFocus(messages []conversation.Message) string {
	if len(messages) == 0 {
		return ""
	}

	// Only use recent user turns to avoid stale summary/assistant text
	// continuously pulling irrelevant operational skills into each request.
	userFocus := make([]string, 0, maxSkillFocusUserMessages)
	for i := len(messages) - 1; i >= 0 && len(userFocus) < maxSkillFocusUserMessages; i-- {
		msg := messages[i]
		if msg.Role != "user" {
			continue
		}
		if v := strings.TrimSpace(msg.Content); v != "" {
			userFocus = append(userFocus, v)
		}
	}
	if len(userFocus) == 0 {
		return ""
	}

	var b strings.Builder
	for i := len(userFocus) - 1; i >= 0; i-- {
		b.WriteString(userFocus[i])
		b.WriteString("\n")
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

func pruneSummaryOverlap(summary string, references ...string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}

	refSegments := buildReferenceSegments(references...)
	if len(refSegments) == 0 {
		return summary
	}

	lines := strings.Split(summary, "\n")
	filtered := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			filtered = append(filtered, "")
			continue
		}
		if isRedundantSummaryLine(trimmed, refSegments) {
			continue
		}
		filtered = append(filtered, line)
	}

	collapsed := make([]string, 0, len(filtered))
	prevBlank := true
	for _, line := range filtered {
		if strings.TrimSpace(line) == "" {
			if prevBlank {
				continue
			}
			collapsed = append(collapsed, "")
			prevBlank = true
			continue
		}
		collapsed = append(collapsed, line)
		prevBlank = false
	}
	return strings.TrimSpace(strings.Join(collapsed, "\n"))
}

func buildReferenceSegments(references ...string) []string {
	seen := make(map[string]struct{}, 64)
	out := make([]string, 0, 64)
	add := func(raw string) {
		normalized := normalizeComparableText(raw)
		if len([]rune(normalized)) < 6 {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	for _, text := range references {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		add(text)
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			add(line)
			if value, ok := extractComparableValue(line); ok {
				add(value)
			}
		}
	}
	return out
}

func isRedundantSummaryLine(line string, refSegments []string) bool {
	lineNorm := normalizeComparableText(line)
	if lineNorm == "" {
		return false
	}

	if isIdentityHeadingLine(lineNorm) {
		return true
	}

	candidates := []string{lineNorm}
	if value, ok := extractComparableValue(line); ok {
		if valueNorm := normalizeComparableText(value); len([]rune(valueNorm)) >= 6 && valueNorm != lineNorm {
			candidates = append(candidates, valueNorm)
		}
	}

	for _, candidate := range candidates {
		if len([]rune(candidate)) < 6 {
			continue
		}
		for _, ref := range refSegments {
			if candidate == ref || strings.Contains(ref, candidate) {
				return true
			}
		}
	}
	return false
}

func isIdentityHeadingLine(normalized string) bool {
	switch normalized {
	case "人格与沟通偏好", "核心身份与风格", "身份与风格", "沟通偏好", "人格设定":
		return true
	default:
		return false
	}
}

func extractComparableValue(line string) (string, bool) {
	cleaned := strings.TrimSpace(line)
	cleaned = strings.TrimLeft(cleaned, "-*• \t")
	cleaned = numberedListPrefixPattern.ReplaceAllString(cleaned, "")
	parts := strings.SplitN(cleaned, "：", 2)
	if len(parts) != 2 {
		parts = strings.SplitN(cleaned, ":", 2)
		if len(parts) != 2 {
			return "", false
		}
	}

	label := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if label == "" || value == "" {
		return "", false
	}
	if len([]rune(label)) > 12 {
		return "", false
	}
	return value, true
}

func normalizeComparableText(text string) string {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return ""
	}
	text = strings.TrimLeft(text, "-*• \t")
	text = numberedListPrefixPattern.ReplaceAllString(text, "")
	text = comparableStripPattern.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
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
