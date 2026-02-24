package agent

import "strings"

func appendCurlHeadersForSettings(command string) string {
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "curl") || !strings.Contains(lower, "/settings/") {
		return command
	}
	if strings.Contains(lower, "--dump-header") || strings.Contains(lower, " -d -") {
		return command
	}
	if strings.Contains(lower, "--data-urlencode") || strings.Contains(lower, "--data ") || strings.Contains(lower, "-d ") {
		return command + " -D -"
	}
	return command
}
