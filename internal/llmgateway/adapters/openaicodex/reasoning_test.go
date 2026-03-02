package openaicodex

import (
	"laughing-barnacle/internal/llmgateway"
	"testing"
)

func TestBuildPayload_IncludesReasoningEffort(t *testing.T) {
	payload := buildPayload(
		llmgateway.CanonicalChatRequest{
			Model:    "gpt-5.3-codex",
			Messages: []llmgateway.CanonicalMessage{{Role: "user", Content: "ping"}},
		},
		"auto",
		"high",
		authContext{Token: "token"},
	)
	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected reasoning payload, got %#v", payload["reasoning"])
	}
	if reasoning["effort"] != "high" {
		t.Fatalf("expected reasoning.effort=high, got %#v", reasoning)
	}
}

func TestBuildPayload_SkipsInvalidReasoningEffort(t *testing.T) {
	payload := buildPayload(
		llmgateway.CanonicalChatRequest{
			Model:    "gpt-5.3-codex",
			Messages: []llmgateway.CanonicalMessage{{Role: "user", Content: "ping"}},
		},
		"auto",
		"extreme",
		authContext{Token: "token"},
	)
	if _, exists := payload["reasoning"]; exists {
		t.Fatalf("did not expect reasoning for invalid effort: %#v", payload["reasoning"])
	}
}
