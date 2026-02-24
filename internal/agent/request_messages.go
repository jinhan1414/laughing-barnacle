package agent

import (
	"fmt"
	"strings"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

func trimMessagesForRequest(
	messages []conversation.Message,
	maxRecent int,
	maxTotalRunes int,
	maxSingleRunes int,
) []conversation.Message {
	if len(messages) == 0 {
		return nil
	}

	start := 0
	if maxRecent > 0 && len(messages) > maxRecent {
		start = len(messages) - maxRecent
	}
	subset := messages[start:]
	rev := make([]conversation.Message, 0, len(subset))
	used := 0
	for i := len(subset) - 1; i >= 0; i-- {
		msg := subset[i]
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if maxSingleRunes > 0 {
			content = trimRunes(content, maxSingleRunes)
		}
		contentRunes := len([]rune(content))
		if maxTotalRunes > 0 && len(rev) > 0 && used+contentRunes > maxTotalRunes {
			break
		}
		if maxTotalRunes > 0 && len(rev) == 0 && contentRunes > maxTotalRunes {
			content = trimRunes(content, maxTotalRunes)
			contentRunes = len([]rune(content))
		}
		msg.Content = content
		rev = append(rev, msg)
		used += contentRunes
	}
	if len(rev) == 0 {
		return nil
	}
	out := make([]conversation.Message, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		out = append(out, rev[i])
	}
	return out
}

func appendHistoryMessagesWithToolCalls(dst []llm.Message, messages []conversation.Message, replayToolCalls bool) []llm.Message {
	if len(messages) == 0 {
		return dst
	}

	out := dst
	for i, msg := range messages {
		out = append(out, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
		// Only replay history tool traces for retrying the latest pending user turn.
		if !replayToolCalls || i != len(messages)-1 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "user") || len(msg.ToolCalls) == 0 {
			continue
		}
		start := 0
		if len(msg.ToolCalls) > maxReplayHistoryToolCalls {
			start = len(msg.ToolCalls) - maxReplayHistoryToolCalls
		}
		for j := start; j < len(msg.ToolCalls); j++ {
			call := msg.ToolCalls[j]
			name := strings.TrimSpace(call.Name)
			if name == "" {
				continue
			}
			callID := strings.TrimSpace(call.ID)
			if callID == "" {
				callID = fmt.Sprintf("history_tool_call_%d_%d_%s", i, j, name)
			}
			args := strings.TrimSpace(call.Arguments)
			if args == "" {
				args = "{}"
			}

			result := strings.TrimSpace(call.Result)
			if errText := strings.TrimSpace(call.Error); errText != "" {
				if result == "" {
					result = "tool execution error: " + errText
				} else {
					result = result + " | tool execution error: " + errText
				}
			}
			if result == "" {
				continue
			}

			out = append(out, llm.Message{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{
					{
						ID:   callID,
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      name,
							Arguments: trimRunes(args, maxContextMessageRunes),
						},
					},
				},
			})
			out = append(out, llm.Message{
				Role:       "tool",
				ToolCallID: callID,
				Content:    trimRunes(result, maxContextMessageRunes),
			})
		}
	}
	return out
}

func hasPendingUserMessage(messages []conversation.Message) bool {
	if len(messages) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(messages[len(messages)-1].Role), "user")
}

func latestUserMessageText(messages []conversation.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}
