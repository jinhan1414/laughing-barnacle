package agent

import (
	"context"
	"fmt"
	"laughing-barnacle/internal/llm"
	"strings"
	"time"
)

func a2aBuiltinToolDefinitions() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		a2aRegisterToolDefinition(),
		a2aSendToolDefinition(),
		a2aGetToolDefinition(),
		a2aCancelToolDefinition(),
	}
}

func a2aRegisterToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinA2ARegisterToolName,
			Description: "Register or update one A2A agent connection in local registry.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_card_url":  map[string]any{"type": "string"},
					"agent_card_json": map[string]any{"type": "string"},
					"alias":           map[string]any{"type": "string"},
					"description":     map[string]any{"type": "string"},
					"endpoint":        map[string]any{"type": "string"},
					"auth_token":      map[string]any{"type": "string"},
					"enabled":         map[string]any{"type": "boolean"},
				},
				"additionalProperties": false,
			},
		},
	}
}

func a2aSendToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinA2ASendToolName,
			Description: "Send one message task to a registered A2A agent.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_id":   map[string]any{"type": "string"},
					"message":    map[string]any{"type": "string"},
					"session_id": map[string]any{"type": "string"},
					"metadata":   map[string]any{"type": "object"},
				},
				"required":             []string{"agent_id", "message"},
				"additionalProperties": false,
			},
		},
	}
}

func a2aGetToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinA2AGetToolName,
			Description: "Get one existing A2A task status by task id.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_id": map[string]any{"type": "string"},
					"task_id":  map[string]any{"type": "string"},
				},
				"required":             []string{"agent_id", "task_id"},
				"additionalProperties": false,
			},
		},
	}
}

func a2aCancelToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinA2ACancelToolName,
			Description: "Cancel one running A2A task by task id.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_id": map[string]any{"type": "string"},
					"task_id":  map[string]any{"type": "string"},
				},
				"required":             []string{"agent_id", "task_id"},
				"additionalProperties": false,
			},
		},
	}
}

func (a *Agent) callA2ARegister(ctx context.Context, raw string) (string, error) {
	provider, err := a.requireA2AProvider()
	if err != nil {
		return "", err
	}
	args, err := readToolArguments(raw)
	if err != nil {
		return "", err
	}
	req := A2ARegisterRequest{
		AgentCardURL:  readStringArgument(args, "agent_card_url"),
		AgentCardJSON: readStringArgument(args, "agent_card_json"),
		Alias:         readStringArgument(args, "alias"),
		Description:   readStringArgument(args, "description"),
		Endpoint:      readStringArgument(args, "endpoint"),
		AuthToken:     readStringArgument(args, "auth_token"),
		Enabled:       readBoolArgument(args, "enabled", true),
	}
	if req.AgentCardURL == "" && req.AgentCardJSON == "" && req.Endpoint == "" {
		return "", fmt.Errorf("one of agent_card_url, agent_card_json or endpoint is required")
	}
	agentInfo, err := provider.Register(ctx, req)
	if err != nil {
		return "", err
	}
	return renderA2AAgentDetail(agentInfo), nil
}

func (a *Agent) callA2ASend(ctx context.Context, raw string) (string, error) {
	provider, err := a.requireA2AProvider()
	if err != nil {
		return "", err
	}
	args, err := readToolArguments(raw)
	if err != nil {
		return "", err
	}
	req := A2ASendRequest{
		AgentID:   requireStringArgument(args, "agent_id"),
		Message:   requireStringArgument(args, "message"),
		SessionID: readStringArgument(args, "session_id"),
		Metadata:  readObjectArgument(args, "metadata"),
	}
	if req.AgentID == "" || req.Message == "" {
		return "", fmt.Errorf("agent_id and message are required")
	}
	result, err := provider.Send(ctx, req)
	if err != nil {
		return "", err
	}
	return renderA2ATaskResult(result), nil
}

func (a *Agent) callA2AGetTask(ctx context.Context, raw string) (string, error) {
	query, provider, err := a.parseA2ATaskQuery(raw)
	if err != nil {
		return "", err
	}
	result, err := provider.GetTask(ctx, query)
	if err != nil {
		return "", err
	}
	return renderA2ATaskResult(result), nil
}

func (a *Agent) callA2ACancelTask(ctx context.Context, raw string) (string, error) {
	query, provider, err := a.parseA2ATaskQuery(raw)
	if err != nil {
		return "", err
	}
	result, err := provider.CancelTask(ctx, query)
	if err != nil {
		return "", err
	}
	return renderA2ATaskResult(result), nil
}

func (a *Agent) parseA2ATaskQuery(raw string) (A2ATaskQuery, A2AProvider, error) {
	provider, err := a.requireA2AProvider()
	if err != nil {
		return A2ATaskQuery{}, nil, err
	}
	args, err := readToolArguments(raw)
	if err != nil {
		return A2ATaskQuery{}, nil, err
	}
	query := A2ATaskQuery{
		AgentID: requireStringArgument(args, "agent_id"),
		TaskID:  requireStringArgument(args, "task_id"),
	}
	if query.AgentID == "" || query.TaskID == "" {
		return A2ATaskQuery{}, nil, fmt.Errorf("agent_id and task_id are required")
	}
	return query, provider, nil
}

func (a *Agent) requireA2AProvider() (A2AProvider, error) {
	if a.a2a == nil {
		return nil, fmt.Errorf("a2a provider not configured")
	}
	return a.a2a, nil
}

func renderA2ATaskResult(result A2ATaskResult) string {
	var b strings.Builder
	b.WriteString("agent_id: " + safeOrEmpty(strings.TrimSpace(result.AgentID)) + "\n")
	b.WriteString("task_id: " + safeOrEmpty(strings.TrimSpace(result.TaskID)) + "\n")
	b.WriteString("status: " + safeOrEmpty(strings.TrimSpace(result.Status)) + "\n")
	if text := strings.TrimSpace(result.Message); text != "" {
		b.WriteString("message: " + text + "\n")
	}
	if len(result.Artifacts) > 0 {
		for _, item := range result.Artifacts {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			b.WriteString("artifact: " + item + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func renderA2AAgentDetail(agentInfo A2AAgentDetail) string {
	var b strings.Builder
	b.WriteString("agent_id: " + safeOrEmpty(strings.TrimSpace(agentInfo.ID)) + "\n")
	b.WriteString("name: " + safeOrEmpty(strings.TrimSpace(agentInfo.Name)) + "\n")
	b.WriteString("enabled: " + boolToOnOff(agentInfo.Enabled) + "\n")
	b.WriteString("endpoint: " + safeOrEmpty(strings.TrimSpace(agentInfo.Endpoint)) + "\n")
	if cardURL := strings.TrimSpace(agentInfo.AgentCardURL); cardURL != "" {
		b.WriteString("agent_card_url: " + cardURL + "\n")
	}
	if desc := strings.TrimSpace(agentInfo.Description); desc != "" {
		b.WriteString("description: " + desc + "\n")
	}
	if !agentInfo.UpdatedAt.IsZero() {
		b.WriteString("updated_at: " + agentInfo.UpdatedAt.UTC().Format(time.RFC3339) + "\n")
	}
	return strings.TrimSpace(b.String())
}

func readStringArgument(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func requireStringArgument(args map[string]any, key string) string {
	return readStringArgument(args, key)
}

func readBoolArgument(args map[string]any, key string, fallback bool) bool {
	value, ok := args[key]
	if !ok {
		return fallback
	}
	boolValue, ok := value.(bool)
	if !ok {
		return fallback
	}
	return boolValue
}

func readObjectArgument(args map[string]any, key string) map[string]any {
	value, ok := args[key]
	if !ok {
		return nil
	}
	objectValue, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	if len(objectValue) == 0 {
		return nil
	}
	return objectValue
}

func boolToOnOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
