package openaicodex

import "strings"

const (
	maxResponsesInputTextRunes = 4000
	scratchpadPlaceholder      = "[internal execution scratchpad omitted]"
)

var assistantLeakMarkers = []string{
	"\nI need to inspect files",
	"\n{\"command\":",
	"\nYes \"to=functions",
	"\nNo to=",
	"\nSo yes as model I can set recipient",
	"\nIn this platform",
}

func sanitizeResponsesMessageText(role, content string) string {
	text := strings.TrimSpace(content)
	if text == "" {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(role), "assistant") {
		text = stripAssistantLeakSuffix(text)
	}
	return trimRunesForResponses(text, maxResponsesInputTextRunes)
}

func sanitizeFunctionCallOutputText(content string) string {
	text := trimRunesForResponses(strings.TrimSpace(content), maxResponsesInputTextRunes)
	if text == "" {
		return ""
	}
	return text
}

func stripAssistantLeakSuffix(content string) string {
	trimmed := strings.TrimSpace(content)
	cutAt := len(trimmed)
	for _, marker := range assistantLeakMarkers {
		if idx := strings.Index(trimmed, marker); idx >= 0 && idx < cutAt {
			cutAt = idx
		}
	}
	if cutAt == len(trimmed) {
		return trimmed
	}
	head := strings.TrimSpace(trimmed[:cutAt])
	if head == "" {
		return scratchpadPlaceholder
	}
	return head
}

func trimRunesForResponses(text string, limit int) string {
	if limit <= 0 {
		return strings.TrimSpace(text)
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return strings.TrimSpace(string(runes[:limit]))
}
