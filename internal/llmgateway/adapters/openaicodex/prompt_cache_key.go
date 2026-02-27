package openaicodex

import (
	"crypto/sha1"
	"encoding/hex"
	"laughing-barnacle/internal/llmgateway"
	"strings"
)

const chatReplyPurpose = "chat_reply"

func resolvePromptCacheKey(req llmgateway.CanonicalChatRequest, auth authContext) string {
	if !isChatGPTAuth(auth) {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(req.Purpose), chatReplyPurpose) {
		return ""
	}
	model := strings.TrimSpace(strings.ToLower(req.Model))
	if model == "" {
		return ""
	}
	sum := sha1.Sum([]byte(chatReplyPurpose + "|" + model))
	// Keep key stable and short to maximize cache reuse per purpose/model.
	return "lb:" + hex.EncodeToString(sum[:8])
}
