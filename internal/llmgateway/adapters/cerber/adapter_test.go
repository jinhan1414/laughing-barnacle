package cerber

import (
	"context"
	"laughing-barnacle/internal/llm"
	"laughing-barnacle/internal/llmgateway"
	"testing"
)

type fakeLLMClient struct {
	req  llm.ChatRequest
	resp llm.ChatResponse
	err  error
}

func (f *fakeLLMClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	f.req = req
	if f.err != nil {
		return llm.ChatResponse{}, f.err
	}
	return f.resp, nil
}

func TestAdapter_MapsCanonicalRequestAndResponse(t *testing.T) {
	client := &fakeLLMClient{
		resp: llm.ChatResponse{
			Content: "ok",
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: llm.ToolFunctionCall{
						Name:      "bash",
						Arguments: `{"command":"pwd"}`,
					},
				},
			},
			Usage: llm.TokenUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
		},
	}
	adapter := NewWithClient(client)

	resp, err := adapter.Chat(context.Background(), llmgateway.CanonicalChatRequest{
		Purpose:  "chat_reply",
		Model:    "gpt-4o-mini",
		Messages: []llmgateway.CanonicalMessage{{Role: "user", Content: "ping"}},
		Tools: []llmgateway.CanonicalToolDefinition{
			{
				Type: "function",
				Function: llmgateway.CanonicalToolFunctionDefinition{
					Name: "bash",
				},
			},
		},
		Temperature: 0.2,
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}

	if client.req.Model != "gpt-4o-mini" || client.req.Purpose != "chat_reply" {
		t.Fatalf("unexpected mapped request: %+v", client.req)
	}
	if len(client.req.Messages) != 1 || client.req.Messages[0].Content != "ping" {
		t.Fatalf("unexpected request messages: %+v", client.req.Messages)
	}
	if len(client.req.Tools) != 1 || client.req.Tools[0].Function.Name != "bash" {
		t.Fatalf("unexpected request tools: %+v", client.req.Tools)
	}
	if resp.Content != "ok" || resp.Usage.TotalTokens != 3 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Function.Name != "bash" {
		t.Fatalf("unexpected mapped tool calls: %+v", resp.ToolCalls)
	}
}
