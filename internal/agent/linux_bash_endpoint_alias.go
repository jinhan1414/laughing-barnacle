package agent

import "strings"

func normalizeLocalAPIEndpointAliases(command string) string {
	command = strings.ReplaceAll(command, "/api/schedules/list", "/api/schedules")
	return command
}

func localAPIEndpointAliasHint(originalCommand string) string {
	lower := strings.ToLower(originalCommand)
	hints := make([]string, 0, 1)
	if strings.Contains(lower, "/api/schedules/list") {
		hints = append(hints, "已自动更正端点：/api/schedules/list -> /api/schedules")
	}
	if len(hints) == 0 {
		return ""
	}
	return strings.Join(hints, "\n")
}
