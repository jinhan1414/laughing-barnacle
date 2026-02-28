package agent

import (
	"context"
	"fmt"
	"strings"

	"laughing-barnacle/internal/llm"
)

func contextReadToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinContextReadToolName,
			Description: "Read local context index/details through whitelisted resources (skills, a2a, memory, async).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"resource":     map[string]any{"type": "string", "enum": []string{"mcp", "skills", "schedules", "a2a", "memory", "async"}},
					"action":       map[string]any{"type": "string", "enum": []string{"list", "read", "index", "section", "get"}},
					"id":           map[string]any{"type": "string"},
					"path":         map[string]any{"type": "string"},
					"section_id":   map[string]any{"type": "string"},
					"task_id":      map[string]any{"type": "string"},
					"include_logs": map[string]any{"type": "boolean"},
					"log_cursor":   map[string]any{"type": "number"},
					"log_limit":    map[string]any{"type": "number"},
				},
				"required":             []string{"resource", "action"},
				"additionalProperties": false,
			},
		},
	}
}

func (a *Agent) callContextRead(ctx context.Context, raw string) (string, error) {
	args, err := readToolArguments(raw)
	if err != nil {
		return "", err
	}
	resource := strings.ToLower(requireStringArgument(args, "resource"))
	action := strings.ToLower(requireStringArgument(args, "action"))
	if resource == "" || action == "" {
		return "", fmt.Errorf("resource and action are required")
	}

	switch resource {
	case "mcp":
		return a.callContextReadMCP(ctx, action)
	case "skills":
		return a.callContextReadSkills(ctx, action, args)
	case "schedules":
		return a.callContextReadSchedules(ctx, action)
	case "a2a":
		return a.callContextReadA2A(ctx, action, args)
	case "memory":
		return a.callContextReadMemory(ctx, action, args)
	case "async":
		return a.callContextReadAsync(action, args)
	default:
		return "", fmt.Errorf("unsupported resource %q", resource)
	}
}

func (a *Agent) callContextReadMCP(ctx context.Context, action string) (string, error) {
	if action != "list" {
		return "", fmt.Errorf("unsupported action %q for mcp", action)
	}
	return a.doLocalAPIRequest(ctx, "GET", "/api/mcp/services", nil, nil)
}

func (a *Agent) callContextReadSkills(ctx context.Context, action string, args map[string]any) (string, error) {
	switch action {
	case "list":
		return a.doLocalAPIRequest(ctx, "GET", "/api/skills", nil, nil)
	case "read":
		id := requireStringArgument(args, "id")
		if id == "" {
			return "", fmt.Errorf("id is required for skills read")
		}
		return a.doLocalAPIRequest(ctx, "GET", "/api/skills/read", map[string]string{"id": id}, nil)
	default:
		return "", fmt.Errorf("unsupported action %q for skills", action)
	}
}

func (a *Agent) callContextReadSchedules(ctx context.Context, action string) (string, error) {
	if action != "list" {
		return "", fmt.Errorf("unsupported action %q for schedules", action)
	}
	return a.doLocalAPIRequest(ctx, "GET", "/api/schedules", nil, nil)
}

func (a *Agent) callContextReadA2A(ctx context.Context, action string, args map[string]any) (string, error) {
	switch action {
	case "list":
		return a.doLocalAPIRequest(ctx, "GET", "/api/a2a/agents", nil, nil)
	case "read":
		id := requireStringArgument(args, "id")
		if id == "" {
			return "", fmt.Errorf("id is required for a2a read")
		}
		return a.doLocalAPIRequest(ctx, "GET", "/api/a2a/agents/read", map[string]string{"id": id}, nil)
	default:
		return "", fmt.Errorf("unsupported action %q for a2a", action)
	}
}

func (a *Agent) callContextReadMemory(ctx context.Context, action string, args map[string]any) (string, error) {
	path := requireStringArgument(args, "path")
	switch action {
	case "index":
		if path == "" {
			return "", fmt.Errorf("path is required for memory index")
		}
		return a.doLocalAPIRequest(ctx, "GET", "/api/memory/index", map[string]string{"path": path}, nil)
	case "read":
		if path == "" {
			return "", fmt.Errorf("path is required for memory read")
		}
		return a.doLocalAPIRequest(ctx, "GET", "/api/memory/read", map[string]string{"path": path}, nil)
	case "section":
		sectionID := requireStringArgument(args, "section_id")
		if path == "" || sectionID == "" {
			return "", fmt.Errorf("path and section_id are required for memory section")
		}
		return a.doLocalAPIRequest(ctx, "GET", "/api/memory/section", map[string]string{
			"path":       path,
			"section_id": sectionID,
		}, nil)
	default:
		return "", fmt.Errorf("unsupported action %q for memory", action)
	}
}

func (a *Agent) callContextReadAsync(action string, args map[string]any) (string, error) {
	if a.asyncTasks == nil {
		return "", fmt.Errorf("async task manager not configured")
	}
	switch action {
	case "list":
		lines := a.asyncTasks.ListIndexLines(20, a.nowFn())
		return marshalNativeAPIResult(nativeAPIResult{
			StatusCode: 200,
			Method:     "LOCAL",
			Path:       "async:list",
			Data: map[string]any{
				"tasks": lines,
			},
		})
	case "get":
		taskID := requireStringArgument(args, "task_id")
		if taskID == "" {
			return "", fmt.Errorf("task_id is required for async get")
		}
		task, err := a.asyncTasks.Get(AsyncTaskGetInput{
			TaskID:      taskID,
			IncludeLogs: readBoolArgument(args, "include_logs"),
			LogCursor:   readIntArgument(args, "log_cursor"),
			LogLimit:    readIntArgument(args, "log_limit"),
		})
		if err != nil {
			return "", err
		}
		return marshalNativeAPIResult(nativeAPIResult{
			StatusCode: 200,
			Method:     "LOCAL",
			Path:       "async:get",
			Data:       task,
		})
	default:
		return "", fmt.Errorf("unsupported action %q for async", action)
	}
}
