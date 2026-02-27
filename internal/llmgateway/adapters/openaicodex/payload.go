package openaicodex

import (
	"encoding/json"
	"fmt"
	"laughing-barnacle/internal/llmgateway"
	"strings"
)

const defaultCodexInstructions = "You are a helpful assistant."

func buildPayload(
	req llmgateway.CanonicalChatRequest,
	fallbackTransport string,
	auth authContext,
) map[string]any {
	instructions, inputMessages := splitInstructions(req.Messages)
	chatGPTAuth := isChatGPTAuth(auth)
	if chatGPTAuth && instructions == "" {
		instructions = defaultCodexInstructions
	}
	transport := resolveTransport(req.Options.Transport, fallbackTransport)
	payload := map[string]any{
		"model": req.Model,
		"input": toResponsesInput(inputMessages),
	}
	if instructions != "" {
		payload["instructions"] = instructions
	}
	if !chatGPTAuth && transport != "" {
		payload["transport"] = transport
	}
	if !chatGPTAuth {
		payload["temperature"] = req.Temperature
	}
	if chatGPTAuth {
		payload["stream"] = true
	}
	if len(req.Tools) > 0 {
		payload["tools"] = toResponsesTools(req.Tools)
	}
	payload["store"] = resolveStore(req.Options.Store)
	return payload
}

func splitInstructions(messages []llmgateway.CanonicalMessage) (string, []llmgateway.CanonicalMessage) {
	if len(messages) == 0 {
		return "", nil
	}
	instructions := make([]string, 0, 2)
	input := make([]llmgateway.CanonicalMessage, 0, len(messages))
	for _, msg := range messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "system") {
			if content := strings.TrimSpace(msg.Content); content != "" {
				instructions = append(instructions, content)
			}
			continue
		}
		input = append(input, msg)
	}
	return strings.TrimSpace(strings.Join(instructions, "\n\n")), input
}

func resolveStore(store *bool) bool {
	if store != nil {
		return *store
	}
	return false
}

func toResponsesInput(messages []llmgateway.CanonicalMessage) []map[string]any {
	input := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "" {
			role = "user"
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			content = extractToolCallSummary(msg.ToolCalls)
		}
		if content == "" {
			continue
		}
		input = append(input, map[string]any{
			"role": role,
			"content": []map[string]any{
				{"type": contentTypeForRole(role), "text": content},
			},
		})
	}
	return input
}

func contentTypeForRole(role string) string {
	if role == "assistant" {
		return "output_text"
	}
	return "input_text"
}

func extractToolCallSummary(calls []llmgateway.CanonicalToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	encoded, err := json.Marshal(calls)
	if err != nil {
		return ""
	}
	return "tool_calls=" + string(encoded)
}

func toResponsesTools(tools []llmgateway.CanonicalToolDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type":        "function",
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
			"parameters":  tool.Function.Parameters,
		})
	}
	return out
}

func resolveTransport(runtimeTransport *string, fallback string) string {
	if runtimeTransport != nil && strings.TrimSpace(*runtimeTransport) != "" {
		return strings.TrimSpace(*runtimeTransport)
	}
	if strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback)
	}
	return "auto"
}

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
	return llmgateway.CanonicalTokenUsage{
		PromptTokens:     maxInt(prompt, 0),
		CompletionTokens: maxInt(completion, 0),
		TotalTokens:      maxInt(total, 0),
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
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
