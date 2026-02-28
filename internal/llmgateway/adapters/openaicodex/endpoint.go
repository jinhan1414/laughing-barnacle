package openaicodex

import "strings"

const (
	defaultOpenAIResponsesEndpoint = "https://api.openai.com/v1/responses"
	defaultCodexResponsesEndpoint  = "https://chatgpt.com/backend-api/codex/responses"
)

func resolveEndpointURL(baseURL string, auth authContext) string {
	trimmed := strings.TrimSpace(baseURL)
	if isChatGPTAuth(auth) {
		return resolveChatGPTEndpoint(trimmed)
	}
	return resolveOpenAIEndpoint(trimmed)
}

func isChatGPTAuth(auth authContext) bool {
	return strings.EqualFold(strings.TrimSpace(auth.AuthMode), authModeChatGPT)
}

func resolveOpenAIEndpoint(baseURL string) string {
	if baseURL == "" {
		return defaultOpenAIResponsesEndpoint
	}
	normalized := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(normalized, "/v1/responses") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1") {
		return normalized + "/responses"
	}
	return normalized + "/v1/responses"
}

func resolveChatGPTEndpoint(baseURL string) string {
	if baseURL == "" || strings.Contains(strings.ToLower(baseURL), "api.openai.com") {
		return defaultCodexResponsesEndpoint
	}
	normalized := strings.TrimRight(baseURL, "/")
	lower := strings.ToLower(normalized)
	if strings.Contains(lower, "/backend-api/codex/responses") {
		return normalized
	}
	if strings.Contains(lower, "/backend-api") {
		return normalized + "/codex/responses"
	}
	return normalized
}
