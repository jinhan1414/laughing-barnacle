package agent

import "strings"

func renderA2ATaskResult(result A2ATaskResult) string {
	var b strings.Builder
	b.WriteString("agent_id: " + safeOrEmpty(strings.TrimSpace(result.AgentID)) + "\n")
	b.WriteString("task_id: " + safeOrEmpty(strings.TrimSpace(result.TaskID)) + "\n")
	b.WriteString("status: " + safeOrEmpty(strings.TrimSpace(result.Status)) + "\n")
	if text := strings.TrimSpace(result.Message); text != "" {
		b.WriteString("message: " + text + "\n")
	}
	for _, item := range result.Artifacts {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		b.WriteString("artifact: " + item + "\n")
	}
	return strings.TrimSpace(b.String())
}
