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

func TestToResponsesInput_AssistantUsesOutputText(t *testing.T) {
	input := toResponsesInput([]llmgateway.CanonicalMessage{
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "world"},
	})
	if len(input) != 2 {
		t.Fatalf("expected 2 input items, got %d", len(input))
	}

	first, ok := input[0]["content"].([]map[string]any)
	if !ok || len(first) == 0 {
		t.Fatalf("invalid first content shape: %#v", input[0]["content"])
	}
	if first[0]["type"] != "output_text" {
		t.Fatalf("assistant content type should be output_text, got %v", first[0]["type"])
	}

	second, ok := input[1]["content"].([]map[string]any)
	if !ok || len(second) == 0 {
		t.Fatalf("invalid second content shape: %#v", input[1]["content"])
	}
	if second[0]["type"] != "input_text" {
		t.Fatalf("user content type should be input_text, got %v", second[0]["type"])
	}
}
