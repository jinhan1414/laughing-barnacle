package web

import (
	"net/url"
	"strings"
)

func buildA2ABaseURL(endpoint string) string {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return ""
	}
	if strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func buildA2AHealthURL(endpoint string) string {
	base := buildA2ABaseURL(endpoint)
	if base == "" {
		return ""
	}
	return base + "/healthz"
}

func buildA2ADebugTasksURL(endpoint string) string {
	base := buildA2ABaseURL(endpoint)
	if base == "" {
		return ""
	}
	return base + "/debug/tasks"
}

func buildA2ADebugTaskURL(endpoint, taskID string) string {
	base := buildA2ABaseURL(endpoint)
	taskID = strings.TrimSpace(taskID)
	if base == "" || taskID == "" {
		return ""
	}
	return base + "/debug/tasks/" + url.PathEscape(taskID)
}
