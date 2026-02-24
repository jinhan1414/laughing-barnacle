package conversation

import (
	"strings"
	"time"
)

func cloneMessages(in []Message) []Message {
	if len(in) == 0 {
		return nil
	}
	out := make([]Message, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].ToolCalls = cloneToolCalls(in[i].ToolCalls)
		out[i].Usage = normalizeTokenUsage(in[i].Usage)
	}
	return out
}

func cloneToolCalls(in []ToolCall) []ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]ToolCall, len(in))
	copy(out, in)
	return out
}

func cloneEvents(in []Event) []Event {
	if len(in) == 0 {
		return nil
	}
	out := make([]Event, len(in))
	copy(out, in)
	return out
}

func normalizeToolCalls(in []ToolCall) []ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(in))
	for _, call := range in {
		call.ID = strings.TrimSpace(call.ID)
		call.Name = strings.TrimSpace(call.Name)
		call.Arguments = strings.TrimSpace(call.Arguments)
		call.Result = strings.TrimSpace(call.Result)
		call.Error = strings.TrimSpace(call.Error)
		if call.Name == "" {
			call.Name = "(unknown)"
		}
		if call.Arguments == "" {
			call.Arguments = "{}"
		}
		if call.CreatedAt.IsZero() {
			call.CreatedAt = time.Now()
		}
		out = append(out, call)
	}
	return out
}

func normalizeEvents(in []Event) []Event {
	if len(in) == 0 {
		return nil
	}
	out := make([]Event, 0, len(in))
	for _, evt := range in {
		evt.Type = strings.TrimSpace(evt.Type)
		evt.Content = strings.TrimSpace(evt.Content)
		if evt.Type == "" || evt.Content == "" {
			continue
		}
		if evt.CreatedAt.IsZero() {
			evt.CreatedAt = time.Now()
		}
		out = append(out, evt)
	}
	if len(out) > maxArchiveEvents {
		out = append([]Event(nil), out[len(out)-maxArchiveEvents:]...)
	}
	return out
}

func normalizeTokenUsage(in *TokenUsage) *TokenUsage {
	if in == nil {
		return nil
	}
	usage := TokenUsage{
		PromptTokens:     max(0, in.PromptTokens),
		CompletionTokens: max(0, in.CompletionTokens),
		TotalTokens:      max(0, in.TotalTokens),
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return &usage
}

func normalizeArchiveText(text string, maxRunes int) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")
	text = strings.Join(strings.Fields(text), " ")
	return trimRunesString(text, maxRunes)
}

func sanitizeField(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "|", "/")
	v = strings.ReplaceAll(v, "\n", " ")
	v = strings.Join(strings.Fields(v), " ")
	if v == "" {
		return "(none)"
	}
	return v
}

func trimRunesString(input string, max int) string {
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
