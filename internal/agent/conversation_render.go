package agent

import (
	"fmt"
	"laughing-barnacle/internal/conversation"
	"strings"
)

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
