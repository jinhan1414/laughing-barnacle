package agent

import (
	"strings"

	"laughing-barnacle/internal/localapi"
)

func (a *Agent) localAPIBaseURL() string {
	baseURL := strings.TrimSpace(a.cfg.LocalAPIBaseURL)
	if baseURL == "" {
		return localapi.DefaultBaseURL
	}
	return strings.TrimRight(baseURL, "/")
}
