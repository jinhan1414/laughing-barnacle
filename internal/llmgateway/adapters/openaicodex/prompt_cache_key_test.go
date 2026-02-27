package openaicodex

import (
	"laughing-barnacle/internal/llmgateway"
	"testing"
)

func TestResolvePromptCacheKey_ChatReplyWithChatGPTAuth(t *testing.T) {
	req := llmgateway.CanonicalChatRequest{
		Purpose: "chat_reply",
		Model:   "gpt-5.3-codex",
	}
	auth := authContext{Token: "token", AuthMode: authModeChatGPT}
	k1 := resolvePromptCacheKey(req, auth)
	k2 := resolvePromptCacheKey(req, auth)
	if k1 == "" {
		t.Fatalf("expected non-empty cache key")
	}
	if k1 != k2 {
		t.Fatalf("cache key should be stable, got %q and %q", k1, k2)
	}
}

func TestResolvePromptCacheKey_EmptyForNonChatReplyPurpose(t *testing.T) {
	req := llmgateway.CanonicalChatRequest{
		Purpose: "memory_extract",
		Model:   "gpt-5.3-codex",
	}
	auth := authContext{Token: "token", AuthMode: authModeChatGPT}
	if got := resolvePromptCacheKey(req, auth); got != "" {
		t.Fatalf("expected empty cache key for non-chat_reply purpose, got %q", got)
	}
}

func TestResolvePromptCacheKey_EmptyForNonChatGPTAuth(t *testing.T) {
	req := llmgateway.CanonicalChatRequest{
		Purpose: "chat_reply",
		Model:   "gpt-5.3-codex",
	}
	auth := authContext{Token: "token"}
	if got := resolvePromptCacheKey(req, auth); got != "" {
		t.Fatalf("expected empty cache key for non-chatgpt auth, got %q", got)
	}
}
