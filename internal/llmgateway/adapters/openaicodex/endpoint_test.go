package openaicodex

import "testing"

func TestResolveEndpointURL_ChatGPTAuthSwitchesFromOpenAIBase(t *testing.T) {
	got := resolveEndpointURL("https://api.openai.com", authContext{
		AuthMode: authModeChatGPT,
		Token:    "x",
	})
	if got != defaultCodexResponsesEndpoint {
		t.Fatalf("expected chatgpt endpoint, got %q", got)
	}
}

func TestResolveEndpointURL_OpenAIAuthUsesResponsesPath(t *testing.T) {
	got := resolveEndpointURL("https://api.openai.com", authContext{Token: "x"})
	if got != defaultOpenAIResponsesEndpoint {
		t.Fatalf("expected openai responses endpoint, got %q", got)
	}
}
