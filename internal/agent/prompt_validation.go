package agent

import (
	"laughing-barnacle/internal/agentprompt"
	"strings"
)

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
	if agentprompt.ContainsDeprecatedSystemPromptSections(systemPrompt) {
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
