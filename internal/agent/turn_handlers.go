package agent

import (
	"context"
	"fmt"
	"strings"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

func (a *Agent) CompressContextNow(ctx context.Context) (string, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store == nil {
		return "", false, fmt.Errorf("conversation store not initialized")
	}

	summary, messages := a.store.Snapshot()
	if strings.TrimSpace(summary) == "" && len(messages) == 0 {
		return "", false, nil
	}

	// Manual compression is expected to aggressively reduce request context.
	// Keep recent messages only for autonomous compression path.
	compressed, err := a.applyCompressionWithKeepLocked(ctx, summary, messages, 0)
	if err != nil {
		return "", false, err
	}
	return compressed, true, nil
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

func (a *Agent) appendTurnToMemoryLocked(userText, assistantText string, toolCalls []conversation.ToolCall) {
	if a == nil || a.memory == nil {
		return
	}
	userText = strings.TrimSpace(userText)
	assistantText = strings.TrimSpace(assistantText)
	if userText == "" && assistantText == "" {
		return
	}
	_ = a.memory.AppendTurn(userText, assistantText, toolCalls, a.nowFn().UTC())
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
	reply, toolCalls, usage, err := a.generateReply(ctx, messages)
	_ = a.store.SetLatestUserToolCalls(toolCalls)
	if err != nil {
		return "", err
	}

	reply = sanitizeLLMReply(reply)
	a.store.AppendAssistant(reply, usage)
	a.appendTurnToMemoryLocked(text, reply, toolCalls)
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

	reply, toolCalls, usage, err := a.generateReply(ctx, messages)
	_ = a.store.SetLatestUserToolCalls(toolCalls)
	if err != nil {
		return "", err
	}

	pendingUser := strings.TrimSpace(messages[len(messages)-1].Content)
	reply = sanitizeLLMReply(reply)
	a.store.AppendAssistant(reply, usage)
	a.appendTurnToMemoryLocked(pendingUser, reply, toolCalls)
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
