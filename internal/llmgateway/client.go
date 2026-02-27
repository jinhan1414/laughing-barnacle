package llmgateway

import (
	"context"
	"fmt"
	"laughing-barnacle/internal/llm"
	"strings"
)

type Adapter interface {
	Name() string
	Chat(ctx context.Context, req CanonicalChatRequest) (CanonicalChatResponse, error)
}

type Config struct {
	DefaultProvider string
	DefaultModel    string
}

type Client struct {
	defaultProvider string
	defaultModel    string
	adapters        map[string]Adapter
}

func NewClient(cfg Config, adapters ...Adapter) (*Client, error) {
	normalizedDefaultProvider := strings.ToLower(strings.TrimSpace(cfg.DefaultProvider))
	if normalizedDefaultProvider == "" {
		return nil, &Error{
			Code:    ErrorCodeInvalidRequest,
			Message: "default provider is required",
		}
	}

	registry := make(map[string]Adapter, len(adapters))
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(adapter.Name()))
		if name == "" {
			return nil, &Error{
				Code:    ErrorCodeInvalidRequest,
				Message: "adapter name is required",
			}
		}
		if _, exists := registry[name]; exists {
			return nil, &Error{
				Code:    ErrorCodeInvalidRequest,
				Message: fmt.Sprintf("duplicate adapter: %s", name),
			}
		}
		registry[name] = adapter
	}
	if _, ok := registry[normalizedDefaultProvider]; !ok {
		return nil, &Error{
			Code:     ErrorCodeProviderNotRegistered,
			Provider: normalizedDefaultProvider,
			Message:  "default provider is not registered",
		}
	}

	return &Client{
		defaultProvider: normalizedDefaultProvider,
		defaultModel:    strings.TrimSpace(cfg.DefaultModel),
		adapters:        registry,
	}, nil
}

func (c *Client) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	provider, model, err := c.resolveProviderModel(req.Model)
	if err != nil {
		return llm.ChatResponse{}, err
	}

	adapter, ok := c.adapters[provider]
	if !ok {
		return llm.ChatResponse{}, &Error{
			Code:     ErrorCodeProviderNotRegistered,
			Provider: provider,
			Message:  "provider is not registered",
		}
	}

	canonicalReq := toCanonicalRequest(req, provider, model)
	canonicalResp, err := adapter.Chat(ctx, canonicalReq)
	if err != nil {
		return llm.ChatResponse{}, WrapProviderError(provider, err)
	}
	return fromCanonicalResponse(canonicalResp), nil
}

func (c *Client) resolveProviderModel(modelRef string) (string, string, error) {
	raw := strings.TrimSpace(modelRef)
	if raw == "" {
		if c.defaultModel == "" {
			return "", "", &Error{
				Code:    ErrorCodeInvalidRequest,
				Message: "model is required",
			}
		}
		return c.defaultProvider, c.defaultModel, nil
	}
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) == 2 {
		provider := strings.ToLower(strings.TrimSpace(parts[0]))
		model := strings.TrimSpace(parts[1])
		if provider == "" || model == "" {
			return "", "", &Error{
				Code:    ErrorCodeInvalidRequest,
				Message: fmt.Sprintf("invalid model reference: %q", raw),
			}
		}
		return provider, model, nil
	}
	return c.defaultProvider, raw, nil
}
