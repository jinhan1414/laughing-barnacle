package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (c *HTTPClient) ensureSession(ctx context.Context, service Service) (string, error) {
	if sid := c.getSession(service.ID); sid != "" {
		return sid, nil
	}

	initResult, headers, err := c.postRPC(ctx, service, "", rpcRequest{
		JSONRPC: "2.0",
		ID:      c.nextReqID(),
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": c.protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"clientInfo": map[string]any{
				"name":    "laughing-barnacle-agent",
				"version": "1.0.0",
			},
		},
	}, true)
	if err != nil {
		return "", fmt.Errorf("initialize mcp service %q failed: %w", service.ID, err)
	}

	var initPayload struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(initResult, &initPayload)

	sessionID := strings.TrimSpace(headers.Get("Mcp-Session-Id"))
	if sessionID != "" {
		c.setSession(service.ID, sessionID)
	}

	_, _, err = c.postRPC(ctx, service, sessionID, rpcRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		Params:  map[string]any{},
	}, false)
	if err != nil {
		return "", fmt.Errorf("send initialized notification failed: %w", err)
	}

	return sessionID, nil
}

func (c *HTTPClient) postRPC(
	ctx context.Context,
	service Service,
	sessionID string,
	payload rpcRequest,
	expectResponse bool,
) (json.RawMessage, http.Header, error) {
	switch normalizeServiceTransport(service.Transport) {
	case ServiceTransportSSE:
		return c.postRPCSSE(ctx, service, sessionID, payload, expectResponse)
	default:
		return c.postRPCStreamable(ctx, service, sessionID, payload, expectResponse)
	}
}

func (c *HTTPClient) postRPCStreamable(
	ctx context.Context,
	service Service,
	sessionID string,
	payload rpcRequest,
	expectResponse bool,
) (json.RawMessage, http.Header, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal rpc request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, service.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("build rpc request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", c.protocolVersion)
	if service.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+service.AuthToken)
	}
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("send rpc request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, fmt.Errorf("read rpc response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, resp.Header, fmt.Errorf("mcp status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}
	if !expectResponse {
		return nil, resp.Header, nil
	}

	rpcResp, err := decodeRPCResponse(respBytes, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, resp.Header, err
	}
	if rpcResp.Error != nil {
		return nil, resp.Header, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, resp.Header, nil
}

func (c *HTTPClient) postRPCSSE(
	ctx context.Context,
	service Service,
	sessionID string,
	payload rpcRequest,
	expectResponse bool,
) (json.RawMessage, http.Header, error) {
	streamReq, err := http.NewRequestWithContext(ctx, http.MethodGet, service.Endpoint, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build sse request: %w", err)
	}
	streamReq.Header.Set("Accept", "text/event-stream")
	streamReq.Header.Set("MCP-Protocol-Version", c.protocolVersion)
	if service.AuthToken != "" {
		streamReq.Header.Set("Authorization", "Bearer "+service.AuthToken)
	}
	if sessionID != "" {
		streamReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	streamResp, err := c.http.Do(streamReq)
	if err != nil {
		return nil, nil, fmt.Errorf("open sse stream: %w", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(streamResp.Body)
		return nil, streamResp.Header, fmt.Errorf("mcp status %d: %s", streamResp.StatusCode, strings.TrimSpace(string(body)))
	}

	reader := bufio.NewReader(streamResp.Body)
	postEndpoint := service.Endpoint
	for {
		event, readErr := readSSEEvent(reader)
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, streamResp.Header, fmt.Errorf("read sse event: %w", readErr)
		}
		if strings.EqualFold(strings.TrimSpace(event.Name), "endpoint") {
			resolved, resolveErr := resolveSSEEndpoint(service.Endpoint, strings.TrimSpace(event.Data))
			if resolveErr != nil {
				return nil, streamResp.Header, resolveErr
			}
			postEndpoint = resolved
			break
		}
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, streamResp.Header, fmt.Errorf("marshal rpc request: %w", err)
	}

	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, postEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, streamResp.Header, fmt.Errorf("build rpc request: %w", err)
	}
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Accept", "application/json, text/event-stream")
	postReq.Header.Set("MCP-Protocol-Version", c.protocolVersion)
	if service.AuthToken != "" {
		postReq.Header.Set("Authorization", "Bearer "+service.AuthToken)
	}
	if sessionID != "" {
		postReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	postResp, err := c.http.Do(postReq)
	if err != nil {
		return nil, streamResp.Header, fmt.Errorf("send rpc request: %w", err)
	}
	defer postResp.Body.Close()
	postBytes, err := io.ReadAll(postResp.Body)
	if err != nil {
		return nil, mergeHeaders(postResp.Header, streamResp.Header), fmt.Errorf("read rpc response: %w", err)
	}
	if postResp.StatusCode >= http.StatusBadRequest {
		return nil, mergeHeaders(postResp.Header, streamResp.Header), fmt.Errorf("mcp status %d: %s", postResp.StatusCode, strings.TrimSpace(string(postBytes)))
	}
	if !expectResponse {
		return nil, mergeHeaders(postResp.Header, streamResp.Header), nil
	}

	if len(bytes.TrimSpace(postBytes)) > 0 {
		rpcResp, decodeErr := decodeRPCResponse(postBytes, postResp.Header.Get("Content-Type"))
		if decodeErr == nil {
			if payload.ID == nil || sameRPCID(payload.ID, rpcResp.ID) {
				if rpcResp.Error != nil {
					return nil, mergeHeaders(postResp.Header, streamResp.Header), fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
				}
				return rpcResp.Result, mergeHeaders(postResp.Header, streamResp.Header), nil
			}
		}
	}

	rpcResp, err := waitRPCResponseFromSSE(reader, payload.ID)
	if err != nil {
		return nil, mergeHeaders(postResp.Header, streamResp.Header), err
	}
	if rpcResp.Error != nil {
		return nil, mergeHeaders(postResp.Header, streamResp.Header), fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, mergeHeaders(postResp.Header, streamResp.Header), nil
}
