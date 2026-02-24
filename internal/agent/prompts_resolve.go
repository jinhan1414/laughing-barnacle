package agent

import (
	"strings"
)

func (a *Agent) resolvePromptsLocked() (systemPrompt string, compressionSystemPrompt string) {
	systemPrompt = strings.TrimSpace(a.cfg.SystemPrompt)
	compressionSystemPrompt = strings.TrimSpace(a.cfg.CompressionSystemPrompt)

	if a.prompts != nil {
		if v := strings.TrimSpace(a.prompts.GetSystemPrompt()); v != "" {
			systemPrompt = v
		}
		if v := strings.TrimSpace(a.prompts.GetCompressionSystemPrompt()); v != "" {
			compressionSystemPrompt = v
		}
	}

	return systemPrompt, compressionSystemPrompt
}
