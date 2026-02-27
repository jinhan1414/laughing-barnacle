package openaicodex

import (
	"laughing-barnacle/internal/llmgateway"
	"testing"
)

func TestBuildPayload_ChatGPTAuthAddsDefaultInstructions(t *testing.T) {
	payload := buildPayload(
		llmgateway.CanonicalChatRequest{
			Model:    "gpt-5.3-codex",
			Messages: []llmgateway.CanonicalMessage{{Role: "user", Content: "ping"}},
		},
		"auto",
		authContext{Token: "token", AuthMode: authModeChatGPT},
	)
	got, ok := payload["instructions"].(string)
	if !ok || got == "" {
		t.Fatalf("expected default instructions, got: %v", payload["instructions"])
	}
	if got != defaultCodexInstructions {
		t.Fatalf("unexpected default instructions: %q", got)
	}
}

func TestBuildPayload_OpenAIAuthDoesNotInjectDefaultInstructions(t *testing.T) {
	payload := buildPayload(
		llmgateway.CanonicalChatRequest{
			Model:    "gpt-5.3-codex",
			Messages: []llmgateway.CanonicalMessage{{Role: "user", Content: "ping"}},
		},
		"auto",
		authContext{Token: "token"},
	)
	if _, exists := payload["instructions"]; exists {
		t.Fatalf("did not expect instructions for non-chatgpt auth: %v", payload["instructions"])
	}
}
