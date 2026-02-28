package llmgateway

import "laughing-barnacle/internal/llm"

// CanonicalChatRequest is the provider-agnostic request shape used inside the gateway.
type CanonicalChatRequest struct {
	Purpose     string
	Provider    string
	Model       string
	Messages    []CanonicalMessage
	Tools       []CanonicalToolDefinition
	Temperature float64
	Options     CanonicalRequestOptions
}

type CanonicalRequestOptions struct {
	Transport *string
	Store     *bool
}

type CanonicalMessage struct {
	Role       string
	Content    string
	Name       string
	ToolCallID string
	ToolCalls  []CanonicalToolCall
}

type CanonicalToolDefinition struct {
	Type     string
	Function CanonicalToolFunctionDefinition
}

type CanonicalToolFunctionDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type CanonicalToolCall struct {
	ID       string
	Type     string
	Function CanonicalToolFunctionCall
}

type CanonicalToolFunctionCall struct {
	Name      string
	Arguments string
}

type CanonicalChatResponse struct {
	Content     string
	ToolCalls   []CanonicalToolCall
	Usage       CanonicalTokenUsage
	RawResponse string
}

type CanonicalTokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
}

func toCanonicalRequest(req llm.ChatRequest, provider, model string) CanonicalChatRequest {
	return CanonicalChatRequest{
		Purpose:     req.Purpose,
		Provider:    provider,
		Model:       model,
		Messages:    toCanonicalMessages(req.Messages),
		Tools:       toCanonicalTools(req.Tools),
		Temperature: req.Temperature,
	}
}

func toCanonicalMessages(messages []llm.Message) []CanonicalMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]CanonicalMessage, 0, len(messages))
	for _, msg := range messages {
		out = append(out, CanonicalMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
			ToolCalls:  toCanonicalToolCalls(msg.ToolCalls),
		})
	}
	return out
}

func toCanonicalTools(tools []llm.ToolDefinition) []CanonicalToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	out := make([]CanonicalToolDefinition, 0, len(tools))
	for _, tool := range tools {
		out = append(out, CanonicalToolDefinition{
			Type: tool.Type,
			Function: CanonicalToolFunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		})
	}
	return out
}

func toCanonicalToolCalls(calls []llm.ToolCall) []CanonicalToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]CanonicalToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, CanonicalToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: CanonicalToolFunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return out
}

func fromCanonicalResponse(resp CanonicalChatResponse) llm.ChatResponse {
	return llm.ChatResponse{
		Content:     resp.Content,
		ToolCalls:   fromCanonicalToolCalls(resp.ToolCalls),
		Usage:       fromCanonicalUsage(resp.Usage),
		RawResponse: resp.RawResponse,
	}
}

func fromCanonicalToolCalls(calls []CanonicalToolCall) []llm.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, llm.ToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: llm.ToolFunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return out
}

func fromCanonicalUsage(usage CanonicalTokenUsage) llm.TokenUsage {
	return llm.TokenUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		CachedTokens:     usage.CachedTokens,
	}
}
