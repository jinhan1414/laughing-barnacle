package agent

import (
	"net/url"
	"strings"
)

func extractRedirectErrorFromCurlHeaders(stdout string) string {
	lines := strings.Split(stdout, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if len(line) < len("location:") || !strings.EqualFold(line[:len("location:")], "location:") {
			continue
		}
		location := strings.TrimSpace(line[len("location:"):])
		if location == "" {
			continue
		}
		parsed, err := url.Parse(location)
		if err != nil || parsed == nil {
			continue
		}
		errText := strings.TrimSpace(parsed.Query().Get("error"))
		if errText == "" {
			continue
		}
		return "settings 接口返回错误：" + errText
	}
	return ""
}
