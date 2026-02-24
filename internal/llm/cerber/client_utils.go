package cerber

import (
	"bytes"
	"encoding/json"

	"laughing-barnacle/internal/llm"
	"strings"
)

func prettyJSONForLog(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	var out bytes.Buffer
	if err := json.Indent(&out, trimmed, "", "  "); err == nil {
		return out.String()
	}
	return string(trimmed)
}

func extractContent(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := extractTextFromPart(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func extractTextFromPart(item any) string {
	m, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	text, ok := m["text"].(string)
	if !ok {
		return ""
	}
	return text
}

func normalizeTokenUsage(promptTokens, completionTokens, totalTokens int) llm.TokenUsage {
	if promptTokens < 0 {
		promptTokens = 0
	}
	if completionTokens < 0 {
		completionTokens = 0
	}
	if totalTokens < 0 {
		totalTokens = 0
	}
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}
	return llm.TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}
