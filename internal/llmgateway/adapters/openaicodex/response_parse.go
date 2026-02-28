package openaicodex

import (
	"encoding/json"
	"fmt"
	"laughing-barnacle/internal/llmgateway"
	"strings"
)

func parseResponse(raw []byte) (llmgateway.CanonicalChatResponse, error) {
	var payload codexResponsePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return llmgateway.CanonicalChatResponse{}, fmt.Errorf("decode response: %w", err)
	}

	content := strings.TrimSpace(payload.OutputText)
	if content == "" {
		content = collectOutputText(payload.Output)
	}
	toolCalls := collectToolCalls(payload.Output)
	if len(toolCalls) == 0 {
		if parsed, ok := tryParseToolCallsSummary(content); ok {
			toolCalls = parsed
			content = ""
		}
	}
	if content == "" && len(toolCalls) == 0 {
		return llmgateway.CanonicalChatResponse{}, fmt.Errorf("empty content and tool_calls in response")
	}

	return llmgateway.CanonicalChatResponse{
		Content:     content,
		ToolCalls:   toolCalls,
		Usage:       normalizeUsage(payload.Usage),
		RawResponse: string(raw),
	}, nil
}

func collectOutputText(items []codexOutputItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item.Type != "message" {
			continue
		}
		for _, part := range item.Content {
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, strings.TrimSpace(part.Text))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func collectToolCalls(items []codexOutputItem) []llmgateway.CanonicalToolCall {
	out := make([]llmgateway.CanonicalToolCall, 0)
	for _, item := range items {
		if item.Type != "function_call" {
			continue
		}
		callID := strings.TrimSpace(item.CallID)
		if callID == "" {
			callID = strings.TrimSpace(item.ID)
		}
		out = append(out, llmgateway.CanonicalToolCall{
			ID:   callID,
			Type: "function",
			Function: llmgateway.CanonicalToolFunctionCall{
				Name:      item.Name,
				Arguments: strings.TrimSpace(item.Arguments),
			},
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func tryParseToolCallsSummary(content string) ([]llmgateway.CanonicalToolCall, bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "tool_calls=") {
		return nil, false
	}
	serialized := strings.TrimSpace(strings.TrimPrefix(trimmed, "tool_calls="))
	if serialized == "" {
		return nil, false
	}
	var calls []llmgateway.CanonicalToolCall
	if err := json.Unmarshal([]byte(serialized), &calls); err != nil {
		return nil, false
	}
	out := make([]llmgateway.CanonicalToolCall, 0, len(calls))
	for idx, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		callID := strings.TrimSpace(call.ID)
		if callID == "" {
			callID = fmt.Sprintf("call_%d_%s", idx, name)
		}
		arguments := strings.TrimSpace(call.Function.Arguments)
		if arguments == "" {
			arguments = "{}"
		}
		callType := strings.TrimSpace(call.Type)
		if callType == "" {
			callType = "function"
		}
		out = append(out, llmgateway.CanonicalToolCall{
			ID:   callID,
			Type: callType,
			Function: llmgateway.CanonicalToolFunctionCall{
				Name:      name,
				Arguments: arguments,
			},
		})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func normalizeUsage(usage codexUsage) llmgateway.CanonicalTokenUsage {
	prompt := usage.InputTokens
	if prompt == 0 {
		prompt = usage.PromptTokens
	}
	completion := usage.OutputTokens
	if completion == 0 {
		completion = usage.CompletionTokens
	}
	total := usage.TotalTokens
	if total == 0 {
		total = prompt + completion
	}
	cached := usage.InputTokensDetails.CachedTokens
	if cached == 0 {
		cached = usage.PromptTokensDetails.CachedTokens
	}
	return llmgateway.CanonicalTokenUsage{
		PromptTokens:     maxInt(prompt, 0),
		CompletionTokens: maxInt(completion, 0),
		TotalTokens:      maxInt(total, 0),
		CachedTokens:     maxInt(cached, 0),
	}
}

func maxInt(value, lower int) int {
	if value < lower {
		return lower
	}
	return value
}

type codexResponsePayload struct {
	OutputText string            `json:"output_text"`
	Output     []codexOutputItem `json:"output"`
	Usage      codexUsage        `json:"usage"`
}

type codexOutputItem struct {
	Type      string              `json:"type"`
	ID        string              `json:"id"`
	CallID    string              `json:"call_id"`
	Name      string              `json:"name"`
	Arguments string              `json:"arguments"`
	Content   []codexContentChunk `json:"content"`
}

type codexContentChunk struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexUsage struct {
	InputTokens         int              `json:"input_tokens"`
	OutputTokens        int              `json:"output_tokens"`
	PromptTokens        int              `json:"prompt_tokens"`
	CompletionTokens    int              `json:"completion_tokens"`
	TotalTokens         int              `json:"total_tokens"`
	InputTokensDetails  codexTokenDetail `json:"input_tokens_details"`
	PromptTokensDetails codexTokenDetail `json:"prompt_tokens_details"`
}

type codexTokenDetail struct {
	CachedTokens int `json:"cached_tokens"`
}
