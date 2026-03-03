package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"laughing-barnacle/internal/llm"
)

const (
	builtinAsyncTaskSubmitToolName = "async_task__submit"
	builtinAsyncTaskGetToolName    = "async_task__get"
	builtinAsyncTaskCancelToolName = "async_task__cancel"
)

func asyncTaskBuiltinToolDefinitions() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		asyncTaskSubmitToolDefinition(),
		asyncTaskGetToolDefinition(),
		asyncTaskCancelToolDefinition(),
	}
}

func asyncTaskSubmitToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinAsyncTaskSubmitToolName,
			Description: "Submit one async task. task_type supports generic or a2a. When task_type=a2a, agent_id and agent_input are required.",
			Parameters: strictToolParameters(
				map[string]any{
					"task_type": map[string]any{
						"type":        "string",
						"enum":        []string{asyncTaskTypeGeneric, asyncTaskTypeA2A},
						"description": "Task category.",
					},
					"request": map[string]any{
						"type":        "string",
						"description": "Task request summary for task list brief. Keep it stable and concise; for a2a tasks align with agent_input and avoid turn words like 再次/继续/重新.",
					},
					"agent_id": map[string]any{
						"type":        "string",
						"description": "Required when task_type=a2a.",
					},
					"agent_input": map[string]any{
						"type":        "string",
						"description": "Required when task_type=a2a. Describe the business task directly; do not use dispatch phrasing like 调用 <agent_id>.",
					},
					"dedupe_key": map[string]any{
						"type":        "string",
						"description": "Optional dedupe key.",
					},
					"notify_on_finish": map[string]any{
						"type":        "boolean",
						"description": "Optional completion notification flag.",
					},
					"metadata": map[string]any{
						"type":                 "object",
						"properties":           asyncTaskMetadataSchema(),
						"additionalProperties": false,
						"description":          "Optional metadata object. For codex-local project tasks, include working_dir directly, or provide project_id so the system can derive working_dir. For new projects also set create_project=true and optional relative_path/aliases/project_summary.",
					},
					"session_id_unused": map[string]any{
						"type":        "string",
						"description": "Deprecated compatibility field; normally null.",
					},
				},
				[]string{"task_type", "request"},
				[]string{"agent_id", "agent_input", "dedupe_key", "notify_on_finish", "metadata", "session_id_unused"},
			),
		},
	}
}

func asyncTaskMetadataSchema() map[string]any {
	return map[string]any{
		"working_dir": map[string]any{
			"type":        "string",
			"description": "Absolute or root-relative working directory for codex-local execution.",
		},
		"project_id": map[string]any{
			"type":        "string",
			"description": "Project identifier for deriving working_dir from project index.",
		},
		"create_project": map[string]any{
			"type":        "boolean",
			"description": "When true and project_id is provided, create project directory under root.",
		},
		"relative_path": map[string]any{
			"type":        "string",
			"description": "Optional project relative path used when create_project=true.",
		},
		"aliases": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Optional project aliases for registry.",
		},
		"project_summary": map[string]any{
			"type":        "string",
			"description": "Optional project summary for registry.",
		},
	}
}

func asyncTaskGetToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinAsyncTaskGetToolName,
			Description: "Get async task status/result and optional log window.",
			Parameters: strictToolParameters(
				map[string]any{
					"task_id": map[string]any{
						"type":        "string",
						"description": "Async task id to read.",
					},
					"include_logs": map[string]any{
						"type":        "boolean",
						"description": "Optional log inclusion flag.",
					},
					"log_cursor": map[string]any{
						"type":        "number",
						"description": "Optional log cursor.",
					},
					"log_limit": map[string]any{
						"type":        "number",
						"description": "Optional log page size.",
					},
				},
				[]string{"task_id"},
				[]string{"include_logs", "log_cursor", "log_limit"},
			),
		},
	}
}

func asyncTaskCancelToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinAsyncTaskCancelToolName,
			Description: "Cancel one running async task by task_id.",
			Parameters: strictToolParameters(
				map[string]any{
					"task_id": map[string]any{
						"type":        "string",
						"description": "Async task id to cancel.",
					},
					"reason": map[string]any{
						"type":        "string",
						"description": "Optional cancellation reason.",
					},
				},
				[]string{"task_id"},
				[]string{"reason"},
			),
		},
	}
}

func (a *Agent) callAsyncTaskSubmit(ctx context.Context, raw string) (string, error) {
	if a.asyncTasks == nil {
		return "", fmt.Errorf("async task manager not configured")
	}
	args, err := readToolArguments(raw)
	if err != nil {
		return "", err
	}
	taskType := requireStringArgument(args, "task_type")
	request := requireStringArgument(args, "request")
	if taskType == "" || request == "" {
		return "", fmt.Errorf("task_type and request are required")
	}
	agentID := readStringArgument(args, "agent_id")
	agentInput := readStringArgument(args, "agent_input")
	if strings.EqualFold(strings.TrimSpace(taskType), asyncTaskTypeA2A) {
		request, agentInput = normalizeA2ARequestAndInput(agentID, request, agentInput)
	}
	metadata := readObjectArgument(args, "metadata")
	if strings.EqualFold(agentID, codexLocalAgentID) {
		metadata, err = a.prepareCodexLocalMetadata(metadata)
		if err != nil {
			return "", err
		}
	}
	notifyOnFinish := true
	if v, ok := args["notify_on_finish"].(bool); ok {
		notifyOnFinish = v
	}
	result, err := a.asyncTasks.Submit(ctx, AsyncTaskSubmitInput{
		TaskType:       taskType,
		Request:        request,
		AgentID:        agentID,
		AgentInput:     agentInput,
		DedupeKey:      readStringArgument(args, "dedupe_key"),
		NotifyOnFinish: notifyOnFinish,
		Metadata:       metadata,
	})
	if err != nil {
		return "", err
	}
	return renderAsyncTaskOutput(result, nil), nil
}

func (a *Agent) callAsyncTaskGet(_ context.Context, raw string) (string, error) {
	if a.asyncTasks == nil {
		return "", fmt.Errorf("async task manager not configured")
	}
	args, err := readToolArguments(raw)
	if err != nil {
		return "", err
	}
	taskID := readStringArgument(args, "task_id")
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	includeLogs := false
	if v, ok := args["include_logs"].(bool); ok {
		includeLogs = v
	}
	logCursor := readIntArgument(args, "log_cursor")
	logLimit := readIntArgument(args, "log_limit")
	task, err := a.asyncTasks.Get(AsyncTaskGetInput{
		TaskID:      taskID,
		IncludeLogs: includeLogs,
		LogCursor:   logCursor,
		LogLimit:    logLimit,
	})
	if err != nil {
		return "", err
	}
	logs := task.Logs
	if !includeLogs {
		logs = nil
	}
	return renderAsyncTaskOutput(task, logs), nil
}

func (a *Agent) callAsyncTaskCancel(ctx context.Context, raw string) (string, error) {
	if a.asyncTasks == nil {
		return "", fmt.Errorf("async task manager not configured")
	}
	args, err := readToolArguments(raw)
	if err != nil {
		return "", err
	}
	taskID := readStringArgument(args, "task_id")
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	task, err := a.asyncTasks.Cancel(ctx, AsyncTaskCancelInput{
		TaskID: taskID,
		Reason: readStringArgument(args, "reason"),
	})
	if err != nil {
		return "", err
	}
	return renderAsyncTaskOutput(task, nil), nil
}

func readIntArgument(args map[string]any, key string) int {
	value, ok := args[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return 0
		}
		parsed, _ := strconv.Atoi(v)
		return parsed
	default:
		return 0
	}
}
