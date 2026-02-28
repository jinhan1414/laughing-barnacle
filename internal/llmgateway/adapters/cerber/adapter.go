package cerber

import (
	"context"
	"laughing-barnacle/internal/llm"
	cerberclient "laughing-barnacle/internal/llm/cerber"
	"laughing-barnacle/internal/llmgateway"
)

const providerName = "cerber"

type Adapter struct {
	client llm.Client
}

func New(cfg cerberclient.Config) *Adapter {
	return &Adapter{client: cerberclient.NewClient(cfg)}
}

func NewWithClient(client llm.Client) *Adapter {
	return &Adapter{client: client}
}

func (a *Adapter) Name() string {
	return providerName
}

func (a *Adapter) Chat(ctx context.Context, req llmgateway.CanonicalChatRequest) (llmgateway.CanonicalChatResponse, error) {
	resp, err := a.client.Chat(ctx, llm.ChatRequest{
		Purpose:     req.Purpose,
		Model:       req.Model,
		Messages:    toLLMMessages(req.Messages),
		Tools:       toLLMTools(req.Tools),
		Temperature: req.Temperature,
	})
	if err != nil {
		return llmgateway.CanonicalChatResponse{}, llmgateway.WrapProviderError(providerName, err)
	}
	return llmgateway.CanonicalChatResponse{
		Content:     resp.Content,
		ToolCalls:   toCanonicalToolCalls(resp.ToolCalls),
		Usage:       toCanonicalUsage(resp.Usage),
		RawResponse: resp.RawResponse,
	}, nil
}

func toLLMMessages(messages []llmgateway.CanonicalMessage) []llm.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]llm.Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, llm.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
			ToolCalls:  toLLMToolCalls(msg.ToolCalls),
		})
	}
	return out
}

func toLLMTools(tools []llmgateway.CanonicalToolDefinition) []llm.ToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	out := make([]llm.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		out = append(out, llm.ToolDefinition{
			Type: tool.Type,
			Function: llm.ToolFunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		})
	}
	return out
}

func toLLMToolCalls(calls []llmgateway.CanonicalToolCall) []llm.ToolCall {
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

func toCanonicalToolCalls(calls []llm.ToolCall) []llmgateway.CanonicalToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]llmgateway.CanonicalToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, llmgateway.CanonicalToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: llmgateway.CanonicalToolFunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return out
}

func toCanonicalUsage(usage llm.TokenUsage) llmgateway.CanonicalTokenUsage {
	return llmgateway.CanonicalTokenUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		CachedTokens:     usage.CachedTokens,
	}
}
