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

func TestToResponsesInput_NormalizesToolRoleToUser(t *testing.T) {
	input := toResponsesInput([]llmgateway.CanonicalMessage{
		{Role: "tool", Content: "exit_code: 0"},
	})
	if len(input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(input))
	}
	if got := input[0]["role"]; got != "user" {
		t.Fatalf("tool role should normalize to user, got %v", got)
	}
	content, ok := input[0]["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		t.Fatalf("invalid content shape: %#v", input[0]["content"])
	}
	if content[0]["type"] != "input_text" {
		t.Fatalf("normalized tool content type should be input_text, got %v", content[0]["type"])
	}
	if content[0]["text"] != "exit_code: 0" {
		t.Fatalf("tool result content should be preserved, got %v", content[0]["text"])
	}
}

func TestToResponsesInput_NormalizesUnknownRoleToUser(t *testing.T) {
	input := toResponsesInput([]llmgateway.CanonicalMessage{
		{Role: "observer", Content: "ping"},
	})
	if len(input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(input))
	}
	if got := input[0]["role"]; got != "user" {
		t.Fatalf("unknown role should normalize to user, got %v", got)
	}
}

func TestParseResponse_UsageIncludesCachedTokensFromInputDetails(t *testing.T) {
	raw := []byte(`{
		"output_text":"ok",
		"usage":{
			"input_tokens":100,
			"output_tokens":20,
			"total_tokens":120,
			"input_tokens_details":{"cached_tokens":88}
		}
	}`)
	resp, err := parseResponse(raw)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if resp.Usage.CachedTokens != 88 {
		t.Fatalf("expected cached_tokens=88, got %+v", resp.Usage)
	}
}

func TestParseResponse_UsageCachedTokensFallsBackToPromptDetails(t *testing.T) {
	raw := []byte(`{
		"output_text":"ok",
		"usage":{
			"prompt_tokens":100,
			"completion_tokens":20,
			"total_tokens":120,
			"prompt_tokens_details":{"cached_tokens":66}
		}
	}`)
	resp, err := parseResponse(raw)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if resp.Usage.CachedTokens != 66 {
		t.Fatalf("expected cached_tokens=66, got %+v", resp.Usage)
	}
}
