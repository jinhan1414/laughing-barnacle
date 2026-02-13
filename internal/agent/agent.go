package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

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
}

type ToolProvider interface {
	ListTools(ctx context.Context) ([]llm.ToolDefinition, error)
	CallTool(ctx context.Context, call llm.ToolCall) (string, error)
}

type SkillProvider interface {
	ListEnabledSkillPrompts() []string
}

type Agent struct {
	cfg    Config
	llm    llm.Client
	tools  ToolProvider
	skills SkillProvider
	store  *conversation.Store
	mu     sync.Mutex
}

func New(cfg Config, store *conversation.Store, llmClient llm.Client, tools ToolProvider) *Agent {
	return &Agent{
		cfg:   cfg,
		llm:   llmClient,
		tools: tools,
		store: store,
	}
}

func (a *Agent) SetSkillProvider(provider SkillProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.skills = provider
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
	reply, err := a.generateReply(ctx, messages)
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

	reply, err := a.generateReply(ctx, messages)
	if err != nil {
		return "", err
	}

	reply = strings.TrimSpace(reply)
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
			{Role: "system", Content: a.cfg.CompressionSystemPrompt},
			{Role: "user", Content: prompt.String()},
		},
		Temperature: 0,
	})
	if err != nil {
		return "", fmt.Errorf("compress context failed: %w", err)
	}
	return resp.Content, nil
}

func (a *Agent) generateReply(ctx context.Context, messages []conversation.Message) (string, error) {
	summary, _ := a.store.Snapshot()

	requestMessages := make([]llm.Message, 0, 2+len(messages))
	requestMessages = append(requestMessages, llm.Message{
		Role:    "system",
		Content: a.cfg.SystemPrompt,
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
			return "", fmt.Errorf("generate reply failed: %w", err)
		}
		return resp.Content, nil
	}

	toolDefs, err := a.tools.ListTools(ctx)
	if err != nil {
		toolDefs = nil
	}

	maxRounds := a.cfg.MaxToolCallRounds
	if maxRounds <= 0 {
		maxRounds = 1
	}

	for i := 0; i < maxRounds; i++ {
		resp, err := a.llm.Chat(ctx, llm.ChatRequest{
			Purpose:     "chat_reply",
			Model:       a.cfg.Model,
			Messages:    requestMessages,
			Tools:       toolDefs,
			Temperature: a.cfg.Temperature,
		})
		if err != nil {
			return "", fmt.Errorf("generate reply failed: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
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

	return "", fmt.Errorf("tool call rounds exceeded %d", maxRounds)
}

func renderConversation(messages []conversation.Message) string {
	var b strings.Builder
	for i, msg := range messages {
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, msg.Role, msg.Content))
	}
	return b.String()
}
