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
	if got := resolvePromptCacheKey(req, auth); got != defaultPromptCacheSessionIDMain {
		t.Fatalf("expected %q, got %q", defaultPromptCacheSessionIDMain, got)
	}
}

func TestResolvePromptCacheSessionID_EmptyForNonChatReplyPurpose(t *testing.T) {
	req := llmgateway.CanonicalChatRequest{
		Purpose: "memory_extract",
		Model:   "gpt-5.3-codex",
	}
	auth := authContext{Token: "token", AuthMode: authModeChatGPT}
	if got := resolvePromptCacheSessionID(req, auth); got != "" {
		t.Fatalf("expected empty cache session id for non-chat_reply purpose, got %q", got)
	}
}

func TestResolvePromptCacheSessionID_EmptyForNonChatGPTAuth(t *testing.T) {
	req := llmgateway.CanonicalChatRequest{
		Purpose: "chat_reply",
		Model:   "gpt-5.3-codex",
	}
	auth := authContext{Token: "token"}
	if got := resolvePromptCacheSessionID(req, auth); got != "" {
		t.Fatalf("expected empty cache session id for non-chatgpt auth, got %q", got)
	}
}
