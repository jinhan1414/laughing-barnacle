package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/mcp"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProviderGetTaskRetriesOnEOF(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeRPCRequest(t, r)
		if req.Method != "tasks/get" {
			writeRPCError(t, w, req.ID, "unsupported method")
			return
		}
		current := atomic.AddInt32(&calls, 1)
		if current == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatalf("handler does not support hijack")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack failed: %v", err)
			}
			_ = conn.Close()
			return
		}
		writeRPCResult(t, w, req.ID, `{"kind":"task","id":"task-1","contextId":"ctx-1","status":{"state":"completed"}}`)
	}))
	defer server.Close()

	provider, agentID := newTestProviderWithAgent(t, testProviderAgent{
		Name:     "codex-local",
		Endpoint: server.URL,
		Enabled:  true,
	})
	result, err := provider.GetTask(context.Background(), agent.A2ATaskQuery{
		AgentID: agentID,
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if result.Status != "completed" || result.RawStatus != "completed" {
		t.Fatalf("unexpected status: %#v", result)
	}
	if result.SDKProvider != sdkProviderName || result.SDKMethod != sdkMethodGetTask {
		t.Fatalf("sdk evidence mismatch: %#v", result)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 requests (retry once), got %d", atomic.LoadInt32(&calls))
	}
}

func TestProviderSendDoesNotRetryOnEOF(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeRPCRequest(t, r)
		if req.Method != "message/send" {
			writeRPCError(t, w, req.ID, "unsupported method")
			return
		}
		atomic.AddInt32(&calls, 1)
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("handler does not support hijack")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack failed: %v", err)
		}
		_ = conn.Close()
	}))
	defer server.Close()

	provider, agentID := newTestProviderWithAgent(t, testProviderAgent{
		Name:     "codex-local",
		Endpoint: server.URL,
		Enabled:  true,
	})
	_, err := provider.Send(context.Background(), agent.A2ASendRequest{
		AgentID: agentID,
		Message: "hello",
	})
	if err == nil {
		t.Fatalf("expected send error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "eof") {
		t.Fatalf("expected EOF error, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected send to avoid retry, got %d requests", atomic.LoadInt32(&calls))
	}
}

func TestProviderSendParsesMessageResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeRPCRequest(t, r)
		if req.Method != "message/send" {
			writeRPCError(t, w, req.ID, "unsupported method")
			return
		}
		writeRPCResult(t, w, req.ID, `{"kind":"message","messageId":"msg-1","role":"agent","parts":[{"kind":"text","text":"ok"}]}`)
	}))
	defer server.Close()

	provider, agentID := newTestProviderWithAgent(t, testProviderAgent{
		Name:     "codex-local",
		Endpoint: server.URL,
		Enabled:  true,
	})
	result, err := provider.Send(context.Background(), agent.A2ASendRequest{
		AgentID: agentID,
		Message: "hello",
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if result.Status != "completed" || result.RawStatus != "message" {
		t.Fatalf("unexpected message result status: %#v", result)
	}
	if result.Message != "ok" {
		t.Fatalf("unexpected message result text: %#v", result)
	}
	if result.SDKProvider != sdkProviderName || result.SDKMethod != sdkMethodSendMessage {
		t.Fatalf("unexpected sdk evidence: %#v", result)
	}
}

func TestProviderSendFailsWhenCardResolveFails(t *testing.T) {
	provider, agentID := newTestProviderWithAgent(t, testProviderAgent{
		Name:         "codex-local",
		Endpoint:     "http://127.0.0.1:9091/a2a/rpc",
		AgentCardURL: "http://127.0.0.1:1/.well-known/agent-card.json",
		Enabled:      true,
		Timeout:      300 * time.Millisecond,
	})
	_, err := provider.Send(context.Background(), agent.A2ASendRequest{
		AgentID: agentID,
		Message: "hello",
	})
	if err == nil {
		t.Fatalf("expected resolver error")
	}
	if !strings.Contains(err.Error(), "resolve agent card") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProviderGetTaskPrefersEndpointWhenCardUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeRPCRequest(t, r)
		if req.Method != "tasks/get" {
			writeRPCError(t, w, req.ID, "unsupported method")
			return
		}
		writeRPCResult(t, w, req.ID, `{"kind":"task","id":"task-2","contextId":"ctx-2","status":{"state":"completed"}}`)
	}))
	defer server.Close()

	provider, agentID := newTestProviderWithAgent(t, testProviderAgent{
		Name:         "codex-local",
		Endpoint:     server.URL,
		AgentCardURL: "http://127.0.0.1:1/.well-known/agent-card.json",
		Enabled:      true,
		Timeout:      300 * time.Millisecond,
	})
	result, err := provider.GetTask(context.Background(), agent.A2ATaskQuery{
		AgentID: agentID,
		TaskID:  "task-2",
	})
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestProviderDisablesKeepAlive(t *testing.T) {
	provider, _ := newTestProviderWithAgent(t, testProviderAgent{
		Name:     "codex-local",
		Endpoint: "http://127.0.0.1:9091/a2a/rpc",
		Enabled:  true,
	})
	transport, ok := provider.http.Transport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatalf("expected *http.Transport")
	}
	if !transport.DisableKeepAlives {
		t.Fatalf("expected DisableKeepAlives=true")
	}
}

type testProviderAgent struct {
	Name         string
	Endpoint     string
	AgentCardURL string
	Enabled      bool
	Timeout      time.Duration
}

func newTestProviderWithAgent(t *testing.T, input testProviderAgent) (*Provider, string) {
	t.Helper()
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := mcp.NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	if err := store.UpsertA2AAgent(mcp.A2AAgent{
		Name:         strings.TrimSpace(input.Name),
		Endpoint:     strings.TrimSpace(input.Endpoint),
		AgentCardURL: strings.TrimSpace(input.AgentCardURL),
		Enabled:      input.Enabled,
	}); err != nil {
		t.Fatalf("UpsertA2AAgent failed: %v", err)
	}
	agents := store.ListA2AAgents()
	if len(agents) != 1 {
		t.Fatalf("expected one agent, got %d", len(agents))
	}
	return NewProvider(store, input.Timeout), agents[0].ID
}

type rpcRequest struct {
	ID      string `json:"id"`
	Method  string `json:"method"`
	JSONRPC string `json:"jsonrpc"`
}

func decodeRPCRequest(t *testing.T, r *http.Request) rpcRequest {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Fatalf("expected POST, got %s", r.Method)
	}
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatalf("decode json-rpc request failed: %v", err)
	}
	if req.JSONRPC != "2.0" {
		t.Fatalf("unexpected jsonrpc version: %q", req.JSONRPC)
	}
	return req
}

func writeRPCResult(t *testing.T, w http.ResponseWriter, id, result string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, id, strings.TrimSpace(result)); err != nil {
		t.Fatalf("write rpc result failed: %v", err)
	}
}

func writeRPCError(t *testing.T, w http.ResponseWriter, id, message string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"error":{"code":-32601,"message":%q}}`, id, strings.TrimSpace(message)); err != nil {
		t.Fatalf("write rpc error failed: %v", err)
	}
}
