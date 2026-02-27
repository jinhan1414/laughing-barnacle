package llmgateway

import (
	"context"
	"errors"
	"laughing-barnacle/internal/llm"
	"testing"
)

type stubAdapter struct {
	name      string
	resp      CanonicalChatResponse
	err       error
	callCount int
	lastReq   CanonicalChatRequest
}

func (a *stubAdapter) Name() string {
	return a.name
}

func (a *stubAdapter) Chat(_ context.Context, req CanonicalChatRequest) (CanonicalChatResponse, error) {
	a.callCount++
	a.lastReq = req
	if a.err != nil {
		return CanonicalChatResponse{}, a.err
	}
	return a.resp, nil
}

func TestGatewayClient_RoutesByModelProviderPrefix(t *testing.T) {
	cerber := &stubAdapter{name: "cerber"}
	codex := &stubAdapter{name: "openai-codex", resp: CanonicalChatResponse{Content: "codex-ok"}}
	client, err := NewClient(Config{DefaultProvider: "cerber", DefaultModel: "gpt-4o-mini"}, cerber, codex)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	resp, err := client.Chat(context.Background(), llm.ChatRequest{
		Model: "openai-codex/gpt-5-codex",
		Messages: []llm.Message{
			{Role: "user", Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if resp.Content != "codex-ok" {
		t.Fatalf("unexpected content: %q", resp.Content)
	}
	if codex.callCount != 1 || cerber.callCount != 0 {
		t.Fatalf("unexpected route count codex=%d cerber=%d", codex.callCount, cerber.callCount)
	}
	if codex.lastReq.Provider != "openai-codex" || codex.lastReq.Model != "gpt-5-codex" {
		t.Fatalf("unexpected routed target: provider=%q model=%q", codex.lastReq.Provider, codex.lastReq.Model)
	}
}

func TestGatewayClient_UsesDefaultProviderWhenNoPrefix(t *testing.T) {
	cerber := &stubAdapter{name: "cerber", resp: CanonicalChatResponse{Content: "cerber-ok"}}
	client, err := NewClient(Config{DefaultProvider: "cerber", DefaultModel: "gpt-4o-mini"}, cerber)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	resp, err := client.Chat(context.Background(), llm.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []llm.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if resp.Content != "cerber-ok" {
		t.Fatalf("unexpected content: %q", resp.Content)
	}
	if cerber.lastReq.Provider != "cerber" || cerber.lastReq.Model != "gpt-4o-mini" {
		t.Fatalf("unexpected routed target: provider=%q model=%q", cerber.lastReq.Provider, cerber.lastReq.Model)
	}
}

func TestGatewayClient_ReturnsExplicitErrorForUnknownProvider(t *testing.T) {
	client, err := NewClient(Config{DefaultProvider: "cerber", DefaultModel: "gpt-4o-mini"}, &stubAdapter{name: "cerber"})
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	_, err = client.Chat(context.Background(), llm.ChatRequest{
		Model: "unknown/model",
		Messages: []llm.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err == nil {
		t.Fatalf("expected unknown provider error")
	}
	var gatewayErr *Error
	if !errors.As(err, &gatewayErr) {
		t.Fatalf("expected gateway error, got %T", err)
	}
	if gatewayErr.Code != ErrorCodeProviderNotRegistered {
		t.Fatalf("unexpected error code: %s", gatewayErr.Code)
	}
}

func TestGatewayClient_DoesNotFallbackOnProviderError(t *testing.T) {
	cerber := &stubAdapter{name: "cerber"}
	codex := &stubAdapter{name: "openai-codex", err: errors.New("provider failure")}
	client, err := NewClient(Config{DefaultProvider: "cerber", DefaultModel: "gpt-4o-mini"}, cerber, codex)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	_, err = client.Chat(context.Background(), llm.ChatRequest{
		Model: "openai-codex/gpt-5-codex",
		Messages: []llm.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err == nil {
		t.Fatalf("expected adapter error")
	}
	var gatewayErr *Error
	if !errors.As(err, &gatewayErr) {
		t.Fatalf("expected wrapped gateway error, got %T", err)
	}
	if gatewayErr.Code != ErrorCodeProviderRequestFailed {
		t.Fatalf("unexpected error code: %s", gatewayErr.Code)
	}
	if codex.callCount != 1 || cerber.callCount != 0 {
		t.Fatalf("unexpected call count codex=%d cerber=%d", codex.callCount, cerber.callCount)
	}
}
