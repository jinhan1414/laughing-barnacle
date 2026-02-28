package openaicodex

import (
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
	chatGPTAuth := isChatGPTAuth(auth)
	instructions, normalizedInput := normalizeInstructionsAndInput(req.Messages, chatGPTAuth)
	transport := resolveTransport(req.Options.Transport, fallbackTransport)
	payload := map[string]any{
		"model": req.Model,
		"input": normalizedInput,
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

func normalizeInstructionsAndInput(
	messages []llmgateway.CanonicalMessage,
	chatGPTAuth bool,
) (string, []map[string]any) {
	instructions, inputMessages := splitInstructions(messages)
	if chatGPTAuth && instructions == "" {
		instructions = defaultCodexInstructions
	}
	return instructions, toResponsesInput(inputMessages)
}

func resolveStore(store *bool) bool {
	if store != nil {
		return *store
	}
	return false
}

func toResponsesInput(messages []llmgateway.CanonicalMessage) []map[string]any {
	input := make([]map[string]any, 0, len(messages))
	for msgIdx, msg := range messages {
		if strings.TrimSpace(msg.ToolCallID) != "" {
			output := strings.TrimSpace(msg.Content)
			if output != "" {
				input = append(input, map[string]any{
					"type":    "function_call_output",
					"call_id": strings.TrimSpace(msg.ToolCallID),
					"output":  output,
				})
			}
			continue
		}

		content := strings.TrimSpace(msg.Content)
		role := normalizeCodexInputRole(msg.Role)
		if content != "" {
			input = append(input, map[string]any{
				"role": role,
				"content": []map[string]any{
					{"type": contentTypeForRole(role), "text": content},
				},
			})
		}
		for callIdx, call := range msg.ToolCalls {
			name := strings.TrimSpace(call.Function.Name)
			if name == "" {
				continue
			}
			callID := strings.TrimSpace(call.ID)
			if callID == "" {
				callID = fmt.Sprintf("tool_call_%d_%d_%s", msgIdx, callIdx, name)
			}
			arguments := strings.TrimSpace(call.Function.Arguments)
			if arguments == "" {
				arguments = "{}"
			}
			input = append(input, map[string]any{
				"type":      "function_call",
				"call_id":   callID,
				"name":      name,
				"arguments": arguments,
			})
		}
	}
	return input
}

func normalizeCodexInputRole(role string) string {
	normalized := strings.ToLower(strings.TrimSpace(role))
	switch normalized {
	case "assistant", "system", "developer", "user":
		return normalized
	default:
		return "user"
	}
}

func contentTypeForRole(role string) string {
	if role == "assistant" {
		return "output_text"
	}
	return "input_text"
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
