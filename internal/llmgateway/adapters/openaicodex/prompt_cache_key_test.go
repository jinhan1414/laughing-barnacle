package openaicodex

import (
	"laughing-barnacle/internal/llmgateway"
	"sync"
	"testing"
)

func TestAdapterPromptCacheKey_OnlyAfterRemember(t *testing.T) {
	adapter := &Adapter{promptCacheKey: make(map[string]string), cacheMu: sync.Mutex{}}
	req := llmgateway.CanonicalChatRequest{
		Purpose: "chat_reply",
		Model:   "gpt-5.3-codex",
	}
	auth := authContext{Token: "token", AuthMode: authModeChatGPT, AccountID: "acct-1"}

	if got := adapter.resolvePromptCacheKey(req, auth); got != "" {
		t.Fatalf("expected empty key before remember, got %q", got)
	}
	adapter.rememberPromptCacheKey(req, auth, llmgateway.CanonicalChatResponse{
		RawResponse: `{"prompt_cache_key":"server-key-1"}`,
	})
	if got := adapter.resolvePromptCacheKey(req, auth); got != "server-key-1" {
		t.Fatalf("expected remembered key, got %q", got)
	}
}

func TestAdapterPromptCacheKey_IgnoreInvalidResponseKey(t *testing.T) {
	adapter := &Adapter{promptCacheKey: make(map[string]string), cacheMu: sync.Mutex{}}
	req := llmgateway.CanonicalChatRequest{
		Purpose: "chat_reply",
		Model:   "gpt-5.3-codex",
	}
	auth := authContext{Token: "token", AuthMode: authModeChatGPT}
	adapter.rememberPromptCacheKey(req, auth, llmgateway.CanonicalChatResponse{
		RawResponse: `{"output_text":"ok"}`,
	})
	if got := adapter.resolvePromptCacheKey(req, auth); got != "" {
		t.Fatalf("expected empty key when response has no prompt_cache_key, got %q", got)
	}
}

func TestShouldUsePromptCacheKey(t *testing.T) {
	req := llmgateway.CanonicalChatRequest{
		Purpose: "chat_reply",
		Model:   "gpt-5.3-codex",
	}
	auth := authContext{Token: "token", AuthMode: authModeChatGPT}
	if !shouldUsePromptCacheKey(req, auth) {
		t.Fatalf("expected shouldUsePromptCacheKey=true")
	}
	if shouldUsePromptCacheKey(llmgateway.CanonicalChatRequest{
		Purpose: "memory_extract",
		Model:   "gpt-5.3-codex",
	}, auth) {
		t.Fatalf("expected false for non-chat_reply purpose")
	}
	if shouldUsePromptCacheKey(req, authContext{Token: "token"}) {
		t.Fatalf("expected false for non-chatgpt auth")
	}
}

func TestExtractPromptCacheKey(t *testing.T) {
	if got := extractPromptCacheKey(`{"prompt_cache_key":"abc"}`); got != "abc" {
		t.Fatalf("expected abc, got %q", got)
	}
	if got := extractPromptCacheKey(`{}`); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := extractPromptCacheKey(`not-json`); got != "" {
		t.Fatalf("expected empty for invalid json, got %q", got)
	}
}
