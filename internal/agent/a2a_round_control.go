package agent

import "strings"

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
