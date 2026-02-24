package agent

import (
	"context"
	"fmt"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"strings"
)

func (a *Agent) autonomousCompressionLoop(ctx context.Context) error {
	for i := 0; i < a.cfg.MaxCompressionLoopsPerTurn; i++ {
		summary, messages := a.store.Snapshot()
		if !a.shouldCompress(summary, messages) {
			return nil
		}

		if _, err := a.applyCompressionLocked(ctx, summary, messages); err != nil {
			return err
		}
	}

	return nil
}

func (a *Agent) applyCompressionLocked(ctx context.Context, summary string, messages []conversation.Message) (string, error) {
	return a.applyCompressionWithKeepLocked(ctx, summary, messages, a.cfg.KeepRecentAfterCompression)
}

func (a *Agent) applyCompressionWithKeepLocked(ctx context.Context, summary string, messages []conversation.Message, keepRecent int) (string, error) {
	compressed, err := a.compressContext(ctx, summary, messages)
	if err != nil {
		return "", err
	}
	compressed = strings.TrimSpace(compressed)
	systemPrompt, _ := a.resolvePromptsLocked()
	compressed = pruneSummaryOverlap(compressed, systemPrompt)
	a.store.SetSummaryAndTrim(compressed, keepRecent)
	if compressed != "" {
		a.store.AppendEvent("context_compression", compressed)
	}
	return compressed, nil
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
