package a2a

import (
	"context"
	"encoding/json"
	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/mcp"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProviderGetTaskRetriesOnEOF(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"2","result":{"task":{"id":"task-1","status":{"state":"completed"},"message":"completed"}}}`))
	}))
	defer server.Close()

	provider, agentID := newTestProviderWithEndpoint(t, server.URL)
	result, err := provider.GetTask(context.Background(), agent.A2ATaskQuery{
		AgentID: agentID,
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("expected completed status, got %q", result.Status)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 requests (retry once), got %d", atomic.LoadInt32(&calls))
	}
}

func TestProviderSendDoesNotRetryOnEOF(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	provider, agentID := newTestProviderWithEndpoint(t, server.URL)
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

func TestProviderDisablesKeepAlive(t *testing.T) {
	provider, _ := newTestProviderWithEndpoint(t, "http://127.0.0.1:9091/a2a/rpc")
	transport, ok := provider.http.Transport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatalf("expected *http.Transport")
	}
	if !transport.DisableKeepAlives {
		t.Fatalf("expected DisableKeepAlives=true")
	}
}

func newTestProviderWithEndpoint(t *testing.T, endpoint string) (*Provider, string) {
	t.Helper()
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := mcp.NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	if err := store.UpsertA2AAgent(mcp.A2AAgent{
		Name:     "codex-local",
		Endpoint: endpoint,
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertA2AAgent failed: %v", err)
	}
	agents := store.ListA2AAgents()
	if len(agents) != 1 {
		raw, _ := json.Marshal(agents)
		t.Fatalf("expected one agent, got %s", raw)
	}
	return NewProvider(store, 0), agents[0].ID
}
