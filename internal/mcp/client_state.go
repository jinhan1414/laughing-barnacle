package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
)

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
