package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
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

	mu       sync.Mutex
	sessions map[string]string
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

func (c *HTTPClient) callRPCStdio(ctx context.Context, service Service, method string, params map[string]any) (json.RawMessage, error) {
	command := strings.TrimSpace(service.Command)
	if command == "" {
		return nil, fmt.Errorf("stdio command is required")
	}

	cmd := exec.CommandContext(ctx, command, service.Args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdio stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdio stdout: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start stdio command: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	enc := json.NewEncoder(stdin)
	dec := json.NewDecoder(bufio.NewReader(stdout))

	initID := c.nextReqID()
	if err := enc.Encode(rpcRequest{
		JSONRPC: "2.0",
		ID:      initID,
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
	}); err != nil {
		return nil, fmt.Errorf("write initialize request: %w", err)
	}
	initResp, err := waitRPCResponseFromSTDIO(dec, initID)
	if err != nil {
		if tail := strings.TrimSpace(stderr.String()); tail != "" {
			return nil, fmt.Errorf("read initialize response: %w; stderr: %s", err, tail)
		}
		return nil, fmt.Errorf("read initialize response: %w", err)
	}
	if initResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", initResp.Error.Code, initResp.Error.Message)
	}

	if err := enc.Encode(rpcRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		Params:  map[string]any{},
	}); err != nil {
		return nil, fmt.Errorf("write initialized notification: %w", err)
	}

	reqID := c.nextReqID()
	if err := enc.Encode(rpcRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  method,
		Params:  params,
	}); err != nil {
		return nil, fmt.Errorf("write rpc request: %w", err)
	}

	resp, err := waitRPCResponseFromSTDIO(dec, reqID)
	if err != nil {
		if tail := strings.TrimSpace(stderr.String()); tail != "" {
			return nil, fmt.Errorf("read rpc response: %w; stderr: %s", err, tail)
		}
		return nil, fmt.Errorf("read rpc response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

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

func decodeRPCResponse(respBytes []byte, contentType string) (rpcResponse, error) {
	trimmed := bytes.TrimSpace(respBytes)
	if len(trimmed) == 0 {
		return rpcResponse{}, fmt.Errorf("decode rpc response: empty response")
	}
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") ||
		bytes.HasPrefix(trimmed, []byte("event:")) ||
		bytes.HasPrefix(trimmed, []byte("data:")) {
		return decodeRPCResponseFromSSE(trimmed, nil)
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(trimmed, &rpcResp); err != nil {
		return rpcResponse{}, fmt.Errorf("decode rpc response: %w", err)
	}
	return rpcResp, nil
}

func decodeRPCResponseFromSSE(payload []byte, expectID any) (rpcResponse, error) {
	reader := bufio.NewReader(bytes.NewReader(payload))
	return waitRPCResponseFromSSE(reader, expectID)
}

func waitRPCResponseFromSSE(reader *bufio.Reader, expectID any) (rpcResponse, error) {
	for {
		event, err := readSSEEvent(reader)
		if err != nil {
			if err == io.EOF {
				return rpcResponse{}, fmt.Errorf("decode rpc response: no rpc message in sse stream")
			}
			return rpcResponse{}, fmt.Errorf("decode rpc response: %w", err)
		}

		data := strings.TrimSpace(event.Data)
		if data == "" {
			continue
		}

		var rpcResp rpcResponse
		if unmarshalErr := json.Unmarshal([]byte(data), &rpcResp); unmarshalErr != nil {
			continue
		}
		if expectID != nil && !sameRPCID(expectID, rpcResp.ID) {
			continue
		}
		return rpcResp, nil
	}
}

func waitRPCResponseFromSTDIO(decoder *json.Decoder, expectID any) (rpcResponse, error) {
	for {
		var envelope map[string]json.RawMessage
		if err := decoder.Decode(&envelope); err != nil {
			if err == io.EOF {
				return rpcResponse{}, fmt.Errorf("decode rpc response: eof")
			}
			return rpcResponse{}, fmt.Errorf("decode rpc response: %w", err)
		}

		methodField, hasMethod := envelope["method"]
		if hasMethod {
			var method string
			if err := json.Unmarshal(methodField, &method); err == nil && strings.TrimSpace(method) != "" {
				// Server initiated request/notification; ignore for this lightweight client.
				continue
			}
		}

		idField, hasID := envelope["id"]
		if !hasID {
			continue
		}
		var id any
		_ = json.Unmarshal(idField, &id)
		if expectID != nil && !sameRPCID(expectID, id) {
			continue
		}

		raw, err := json.Marshal(envelope)
		if err != nil {
			return rpcResponse{}, fmt.Errorf("decode rpc response: %w", err)
		}
		var resp rpcResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return rpcResponse{}, fmt.Errorf("decode rpc response: %w", err)
		}
		return resp, nil
	}
}

type sseEvent struct {
	Name string
	Data string
}

func readSSEEvent(reader *bufio.Reader) (sseEvent, error) {
	var event sseEvent
	hasData := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return sseEvent{}, err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if hasData {
				return event, nil
			}
		} else if strings.HasPrefix(line, ":") {
			// ignore comment/heartbeat
		} else if strings.HasPrefix(line, "event:") {
			event.Name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			hasData = true
		} else if strings.HasPrefix(line, "data:") {
			part := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if event.Data == "" {
				event.Data = part
			} else {
				event.Data += "\n" + part
			}
			hasData = true
		}

		if err == io.EOF {
			if hasData {
				return event, nil
			}
			return sseEvent{}, io.EOF
		}
	}
}

func resolveSSEEndpoint(baseEndpoint, eventData string) (string, error) {
	if eventData == "" {
		return "", fmt.Errorf("empty sse endpoint event")
	}
	baseURL, err := url.Parse(baseEndpoint)
	if err != nil {
		return "", fmt.Errorf("parse base endpoint: %w", err)
	}
	ref, err := url.Parse(eventData)
	if err != nil {
		return "", fmt.Errorf("parse sse endpoint: %w", err)
	}
	return baseURL.ResolveReference(ref).String(), nil
}

func sameRPCID(a, b any) bool {
	return strings.TrimSpace(fmt.Sprintf("%v", a)) == strings.TrimSpace(fmt.Sprintf("%v", b))
}

func mergeHeaders(primary, secondary http.Header) http.Header {
	merged := make(http.Header)
	for key, values := range secondary {
		merged[key] = append([]string(nil), values...)
	}
	for key, values := range primary {
		merged[key] = append([]string(nil), values...)
	}
	return merged
}

func (c *HTTPClient) updateSessionFromHeaders(serviceID string, headers http.Header) {
	sid := strings.TrimSpace(headers.Get("Mcp-Session-Id"))
	if sid == "" {
		return
	}
	c.setSession(serviceID, sid)
}

func (c *HTTPClient) nextReqID() int64 {
	return c.reqID.Add(1)
}

func (c *HTTPClient) getSession(serviceID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessions[serviceID]
}

func (c *HTTPClient) setSession(serviceID, sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessions[serviceID] = sessionID
}

func (c *HTTPClient) clearSession(serviceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, serviceID)
}

type rpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
