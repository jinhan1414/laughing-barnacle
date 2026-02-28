package agent

import (
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"strings"
	"time"
)

func sanitizeLLMReply(reply string) string {
	reply = strings.ReplaceAll(reply, "\r\n", "\n")
	var b strings.Builder
	for _, r := range reply {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r >= 32:
			b.WriteRune(r)
		}
	}

	clean := strings.TrimSpace(b.String())
	for strings.Contains(clean, "\"\"\"") {
		clean = strings.ReplaceAll(clean, "\"\"\"", "\"")
	}
	if strings.Count(clean, "```")%2 == 1 {
		clean = strings.ReplaceAll(clean, "```", "")
	}
	return trimRunes(clean, maxAssistantReplyRunes)
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

func buildCurrentDateUserContextPrompt(now time.Time) string {
	now = now.Round(0)
	zoneName, _ := now.Zone()
	zoneName = strings.TrimSpace(zoneName)
	if zoneName == "" {
		zoneName = "Local"
	}
	return runtimeDateContextMarker + "\n" +
		"以下是运行时上下文，不是用户新问题：\n" +
		"时间基准（用于相对时间换算）：当前日期 " +
		now.Format("2006-01-02") +
		"（时区: " + zoneName +
		"）。凡涉及“今天/昨天/最近N天/本周/本月”等时间范围，必须以此为准计算后再查询；如需小时/分钟级当前时间，先调用 bash 查询系统时间。"
}

func removeRuntimeDateContextUserMessages(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return messages
	}
	out := make([]llm.Message, 0, len(messages))
	for _, msg := range messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "user") &&
			strings.Contains(msg.Content, runtimeDateContextMarker) {
			continue
		}
		out = append(out, msg)
	}
	return out
}
