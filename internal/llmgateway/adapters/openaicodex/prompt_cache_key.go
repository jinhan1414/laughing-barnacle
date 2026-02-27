package openaicodex

import (
	"encoding/json"
	"laughing-barnacle/internal/llmgateway"
	"strings"
)

const chatReplyPurpose = "chat_reply"

func (a *Adapter) resolvePromptCacheKey(req llmgateway.CanonicalChatRequest, auth authContext) string {
	if !shouldUsePromptCacheKey(req, auth) {
		return ""
	}
	storeKey := promptCacheStoreKey(req, auth)
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	return a.promptCacheKey[storeKey]
}

func (a *Adapter) rememberPromptCacheKey(
	req llmgateway.CanonicalChatRequest,
	auth authContext,
	resp llmgateway.CanonicalChatResponse,
) {
	if !shouldUsePromptCacheKey(req, auth) {
		return
	}
	cacheKey := extractPromptCacheKey(resp.RawResponse)
	if cacheKey == "" {
		return
	}
	storeKey := promptCacheStoreKey(req, auth)
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	a.promptCacheKey[storeKey] = cacheKey
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

func promptCacheStoreKey(req llmgateway.CanonicalChatRequest, auth authContext) string {
	purpose := strings.ToLower(strings.TrimSpace(req.Purpose))
	model := strings.ToLower(strings.TrimSpace(req.Model))
	accountID := strings.TrimSpace(auth.AccountID)
	return purpose + "|" + model + "|" + accountID
}

func extractPromptCacheKey(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	var payload struct {
		PromptCacheKey string `json:"prompt_cache_key"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.PromptCacheKey)
}
