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
	SystemPrompt               string
	CompressionSystemPrompt    string
}

type Agent struct {
	cfg   Config
	llm   llm.Client
	store *conversation.Store
	mu    sync.Mutex
}

func New(cfg Config, store *conversation.Store, llmClient llm.Client) *Agent {
	return &Agent{
		cfg:   cfg,
		llm:   llmClient,
		store: store,
	}
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

func renderConversation(messages []conversation.Message) string {
	var b strings.Builder
	for i, msg := range messages {
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, msg.Role, msg.Content))
	}
	return b.String()
}
