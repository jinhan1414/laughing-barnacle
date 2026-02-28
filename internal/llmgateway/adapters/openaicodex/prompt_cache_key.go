package openaicodex

import (
	"laughing-barnacle/internal/llmgateway"
	"strings"
)

const (
	chatReplyPurpose                = "chat_reply"
	defaultPromptCacheSessionIDMain = "agent:main:main"
)

func resolvePromptCacheKey(req llmgateway.CanonicalChatRequest, auth authContext) string {
	return resolvePromptCacheSessionID(req, auth)
}

func resolvePromptCacheSessionID(req llmgateway.CanonicalChatRequest, auth authContext) string {
	if !shouldUsePromptCacheKey(req, auth) {
		return ""
	}
	return defaultPromptCacheSessionIDMain
}

func shouldUsePromptCacheKey(req llmgateway.CanonicalChatRequest, auth authContext) bool {
	if !isChatGPTAuth(auth) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(req.Purpose), chatReplyPurpose) {
		return false
	}
	return strings.TrimSpace(req.Model) != ""
}
