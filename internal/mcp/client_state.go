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

func (c *HTTPClient) getStdioSession(serviceID string) *stdioSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stdioSessions[serviceID]
}

func (c *HTTPClient) setStdioSession(serviceID string, session *stdioSession) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stdioSessions[serviceID] = session
}

func (c *HTTPClient) deleteStdioSession(serviceID string) *stdioSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	session := c.stdioSessions[serviceID]
	delete(c.stdioSessions, serviceID)
	return session
}

func (c *HTTPClient) invalidateStdioSession(serviceID string, session *stdioSession) {
	if session == nil {
		return
	}
	c.mu.Lock()
	current := c.stdioSessions[serviceID]
	if current == session {
		delete(c.stdioSessions, serviceID)
	}
	c.mu.Unlock()
	if current == session {
		session.close()
	}
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
