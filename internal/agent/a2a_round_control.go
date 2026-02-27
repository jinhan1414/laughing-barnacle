package agent

import (
	"laughing-barnacle/internal/conversation"
	"strings"
)

func isA2ABuiltinTool(name string) bool {
	switch strings.TrimSpace(name) {
	case builtinA2ASendToolName, builtinA2AGetToolName, builtinA2ACancelToolName:
		return true
	default:
		return false
	}
}

func isA2AInProgressResult(result string) bool {
	text := strings.ToLower(strings.TrimSpace(result))
	return strings.Contains(text, "status: working") || strings.Contains(text, "status: submitted")
}

func parseTaskIDFromA2AResult(result string) string {
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "task_id:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "task_id:"))
		if value != "" {
			return value
		}
	}
	return ""
}

func latestA2AProgressTaskID(calls []conversation.ToolCall) string {
	for i := len(calls) - 1; i >= 0; i-- {
		call := calls[i]
		if !isA2ABuiltinTool(call.Name) {
			continue
		}
		if !isA2AInProgressResult(call.Result) {
			continue
		}
		taskID := parseTaskIDFromA2AResult(call.Result)
		if taskID != "" {
			return taskID
		}
	}
	return ""
}

func latestA2AToolError(calls []conversation.ToolCall) string {
	for i := len(calls) - 1; i >= 0; i-- {
		call := calls[i]
		if !isA2ABuiltinTool(call.Name) {
			continue
		}
		errText := strings.TrimSpace(call.Error)
		if errText != "" {
			return errText
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(call.Result)), "tool execution error:") {
			return strings.TrimSpace(call.Result)
		}
	}
	return ""
}
