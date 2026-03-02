package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultProtocolVersion = "2025-06-18"

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type ToolCallResult struct {
	Content           []ToolContentPart `json:"content,omitempty"`
	StructuredContent any               `json:"structuredContent,omitempty"`
	IsError           bool              `json:"isError,omitempty"`
}

type ToolContentPart struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

type HTTPClient struct {
	http            *http.Client
	protocolVersion string

	reqID atomic.Int64

	mu            sync.Mutex
	sessions      map[string]string
	stdioSessions map[string]*stdioSession
}

func NewHTTPClient(timeout time.Duration, protocolVersion string) *HTTPClient {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if strings.TrimSpace(protocolVersion) == "" {
		protocolVersion = defaultProtocolVersion
	}

	return &HTTPClient{
		http:            &http.Client{Timeout: timeout},
		protocolVersion: protocolVersion,
		sessions:        make(map[string]string),
		stdioSessions:   make(map[string]*stdioSession),
	}
}

func (c *HTTPClient) ListTools(ctx context.Context, service Service) ([]Tool, error) {
	raw, err := c.callRPC(ctx, service, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	var payload struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode tools/list: %w", err)
	}
	return payload.Tools, nil
}

func (c *HTTPClient) CallTool(ctx context.Context, service Service, toolName string, args map[string]any) (ToolCallResult, error) {
	raw, err := c.callRPC(ctx, service, "tools/call", map[string]any{
		"name":      toolName,
		"arguments": args,
	})
	if err != nil {
		return ToolCallResult{}, err
	}

	var result ToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ToolCallResult{}, fmt.Errorf("decode tools/call: %w", err)
	}
	return result, nil
}

func (c *HTTPClient) callRPC(ctx context.Context, service Service, method string, params map[string]any) (json.RawMessage, error) {
	if normalizeServiceTransport(service.Transport) == ServiceTransportStdio {
		return c.callRPCStdio(ctx, service, method, params)
	}

	sessionID, err := c.ensureSession(ctx, service)
	if err != nil {
		return nil, err
	}

	result, headers, err := c.postRPC(ctx, service, sessionID, rpcRequest{
		JSONRPC: "2.0",
		ID:      c.nextReqID(),
		Method:  method,
		Params:  params,
	}, true)
	if err == nil {
		c.updateSessionFromHeaders(service.ID, headers)
		return result, nil
	}

	if sessionID == "" {
		return nil, err
	}

	c.clearSession(service.ID)
	sessionID, reinitErr := c.ensureSession(ctx, service)
	if reinitErr != nil {
		return nil, fmt.Errorf("rpc failed: %v; reinitialize failed: %w", err, reinitErr)
	}
	result, headers, retryErr := c.postRPC(ctx, service, sessionID, rpcRequest{
		JSONRPC: "2.0",
		ID:      c.nextReqID(),
		Method:  method,
		Params:  params,
	}, true)
	if retryErr != nil {
		return nil, fmt.Errorf("rpc failed after session retry: %w", retryErr)
	}
	c.updateSessionFromHeaders(service.ID, headers)
	return result, nil
}
