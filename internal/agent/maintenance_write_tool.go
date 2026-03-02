package agent

import (
	"context"
	"fmt"
	"strings"

	"laughing-barnacle/internal/llm"
)

func maintenanceWriteToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinMaintenanceWriteToolName,
			Description: "Write local maintenance configs with structured JSON payload (mcp, skills, schedules, a2a).",
			Parameters: strictToolParameters(
				map[string]any{
					"resource": map[string]any{
						"type":        "string",
						"enum":        []string{"mcp", "skills", "schedules", "a2a"},
						"description": "Target maintenance namespace.",
					},
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"save", "toggle", "delete", "run", "install"},
						"description": "Mutation type. Resource/action compatibility is validated server-side.",
					},
					"payload": map[string]any{
						"type":        "object",
						"description": "Structured body. toggle requires {id,enabled}; delete/run require {id}; save/install fields vary by resource.",
					},
				},
				[]string{"resource", "action", "payload"},
				nil,
			),
		},
	}
}

func (a *Agent) callMaintenanceWrite(ctx context.Context, raw string) (string, error) {
	args, err := readToolArguments(raw)
	if err != nil {
		return "", err
	}
	resource := strings.ToLower(requireStringArgument(args, "resource"))
	action := strings.ToLower(requireStringArgument(args, "action"))
	payload := readObjectArgument(args, "payload")
	if resource == "" || action == "" {
		return "", fmt.Errorf("resource and action are required")
	}
	if len(payload) == 0 {
		return "", fmt.Errorf("payload is required")
	}

	path, err := resolveMaintenanceWritePath(resource, action)
	if err != nil {
		return "", err
	}
	if err := validateMaintenanceWritePayload(resource, action, payload); err != nil {
		return "", err
	}
	return a.doLocalAPIRequest(ctx, "POST", path, nil, payload)
}

func resolveMaintenanceWritePath(resource string, action string) (string, error) {
	switch resource {
	case "mcp":
		switch action {
		case "save", "toggle", "delete":
			return "/api/mcp/services/" + action, nil
		}
	case "skills":
		switch action {
		case "save", "toggle", "delete", "install":
			return "/api/skills/" + action, nil
		}
	case "schedules":
		switch action {
		case "save", "toggle", "delete", "run":
			return "/api/schedules/" + action, nil
		}
	case "a2a":
		switch action {
		case "save", "toggle", "delete":
			return "/api/a2a/agents/" + action, nil
		}
	}
	return "", fmt.Errorf("unsupported action %q for resource %q", action, resource)
}

func validateMaintenanceWritePayload(resource string, action string, payload map[string]any) error {
	switch action {
	case "toggle":
		return requirePayloadFields(payload, "id", "enabled")
	case "delete", "run":
		return requirePayloadFields(payload, "id")
	case "install":
		if resource != "skills" {
			return fmt.Errorf("install only supports skills resource")
		}
		return requirePayloadFields(payload, "skills_sh_url")
	case "save":
		return validateSavePayload(resource, payload)
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
}

func validateSavePayload(resource string, payload map[string]any) error {
	switch resource {
	case "mcp":
		if err := requirePayloadFields(payload, "name", "transport", "enabled"); err != nil {
			return err
		}
		transport := readPayloadString(payload, "transport")
		switch transport {
		case "streamable_http":
			return requirePayloadFields(payload, "endpoint")
		case "stdio":
			return requirePayloadFields(payload, "command")
		default:
			return fmt.Errorf("mcp save requires transport=streamable_http|stdio")
		}
	case "skills":
		return requirePayloadFields(payload, "id", "name", "description", "prompt", "enabled")
	case "schedules":
		return requirePayloadFields(payload, "id", "name", "description", "action", "cron_expr", "enabled")
	case "a2a":
		if err := requirePayloadFields(payload, "name", "description", "enabled"); err != nil {
			return err
		}
		if readPayloadString(payload, "endpoint") == "" && readPayloadString(payload, "agent_card_url") == "" {
			return fmt.Errorf("a2a save requires endpoint or agent_card_url")
		}
		return nil
	default:
		return fmt.Errorf("unsupported resource %q", resource)
	}
}

func requirePayloadFields(payload map[string]any, fields ...string) error {
	for _, field := range fields {
		value, ok := payload[field]
		if !ok {
			return fmt.Errorf("payload field %q is required", field)
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				return fmt.Errorf("payload field %q must be non-empty", field)
			}
		case nil:
			return fmt.Errorf("payload field %q must not be null", field)
		}
	}
	return nil
}

func readPayloadString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}
