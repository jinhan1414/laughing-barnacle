package llm

import "context"

// Message is a chat message compatible with OpenAI-style chat APIs.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function ToolFunctionDefinition `json:"function"`
}

type ToolFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolFunctionCall `json:"function"`
}

type ToolFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatRequest represents one non-streaming completion request.
type ChatRequest struct {
	Purpose     string           `json:"-"`
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
}

// ChatResponse is the normalized LLM reply.
type ChatResponse struct {
	Content     string
	ToolCalls   []ToolCall
	RawResponse string
}

// Client is the LLM abstraction used by the agent.
type Client interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}
