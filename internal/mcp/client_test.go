package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClient_ListAndCallTool(t *testing.T) {
	var calls []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("MCP-Protocol-Version"); got != "2025-06-18" {
			t.Fatalf("expected MCP-Protocol-Version header, got %q", got)
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		method, _ := req["method"].(string)
		calls = append(calls, method)

		switch method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-1")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
		case "notifications/initialized":
			if got := r.Header.Get("Mcp-Session-Id"); got != "session-1" {
				t.Fatalf("expected session header on initialized, got %q", got)
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if got := r.Header.Get("Mcp-Session-Id"); got != "session-1" {
				t.Fatalf("expected session header on tools/list, got %q", got)
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"weather","description":"query weather","inputSchema":{"type":"object"}}]}}`))
		case "tools/call":
			if got := r.Header.Get("Mcp-Session-Id"); got != "session-1" {
				t.Fatalf("expected session header on tools/call, got %q", got)
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"sunny"}]}}`))
		default:
			t.Fatalf("unexpected method: %s", method)
		}
	}))
	defer ts.Close()

	client := NewHTTPClient(3*time.Second, "")
	service := Service{
		ID:       "weather",
		Name:     "Weather",
		Endpoint: ts.URL,
		Enabled:  true,
	}

	tools, err := client.ListTools(context.Background(), service)
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "weather" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	result, err := client.CallTool(context.Background(), service, "weather", map[string]any{"city": "beijing"})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "sunny" {
		t.Fatalf("unexpected result: %+v", result)
	}

	if len(calls) != 4 {
		t.Fatalf("expected 4 rpc calls, got %d (%v)", len(calls), calls)
	}
}

func TestHTTPClient_StreamableHTTPWithSSEResponse(t *testing.T) {
	var calls []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		method, _ := req["method"].(string)
		calls = append(calls, method)

		switch method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-sse-1")
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2025-06-18\"}}\n\n"))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"tools\":[{\"name\":\"read_wiki\",\"description\":\"read\",\"inputSchema\":{\"type\":\"object\"}}]}}\n\n"))
		default:
			t.Fatalf("unexpected method: %s", method)
		}
	}))
	defer ts.Close()

	client := NewHTTPClient(3*time.Second, "")
	service := Service{
		ID:        "deepwiki",
		Name:      "DeepWiki",
		Endpoint:  ts.URL,
		Transport: "streamableHttp",
		Enabled:   true,
	}

	tools, err := client.ListTools(context.Background(), service)
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "read_wiki" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 rpc calls, got %d (%v)", len(calls), calls)
	}
}
