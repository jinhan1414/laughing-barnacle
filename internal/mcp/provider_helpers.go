package mcp

import (
	"encoding/json"
	"fmt"
	"laughing-barnacle/internal/llm"
	"strings"
)

func toToolDefinition(service Service, tool Tool) (llm.ToolDefinition, toolBinding) {
	prefix := sanitizeName(service.ID)
	toolName := sanitizeName(tool.Name)
	fullName := prefix + "__" + toolName
	if prefix == "" {
		fullName = toolName
	}

	description := strings.TrimSpace(tool.Description)
	if description == "" {
		description = "MCP tool"
	}
	description = fmt.Sprintf("[MCP %s] %s", service.Name, description)

	params := tool.InputSchema
	if params == nil {
		params = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	return llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionDefinition{
				Name:        fullName,
				Description: description,
				Parameters:  params,
			},
		}, toolBinding{
			ServiceID: service.ID,
			ToolName:  tool.Name,
		}
}

func sanitizeName(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "tool"
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func parseToolArguments(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}, nil
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

func renderToolResult(result ToolCallResult) string {
	textParts := make([]string, 0, len(result.Content))
	for _, item := range result.Content {
		if strings.EqualFold(item.Type, "text") && strings.TrimSpace(item.Text) != "" {
			textParts = append(textParts, item.Text)
		}
	}
	if len(textParts) > 0 {
		return strings.Join(textParts, "\n")
	}

	if result.StructuredContent != nil {
		data, err := json.Marshal(result.StructuredContent)
		if err == nil {
			return string(data)
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func bindingExists(bindings map[string]toolBinding, name string) bool {
	_, ok := bindings[name]
	return ok
}

func cloneToolDefs(defs []llm.ToolDefinition) []llm.ToolDefinition {
	out := make([]llm.ToolDefinition, len(defs))
	copy(out, defs)
	return out
}
