package llm

import "context"

// Message is a chat message compatible with OpenAI-style chat APIs.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents one non-streaming completion request.
type ChatRequest struct {
	Purpose     string    `json:"-"`
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
}

// ChatResponse is the normalized LLM reply.
type ChatResponse struct {
	Content     string
	RawResponse string
}

// Client is the LLM abstraction used by the agent.
type Client interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}
