package cerber

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"laughing-barnacle/internal/llm"
	"laughing-barnacle/internal/llmlog"
)

func TestClientChat(t *testing.T) {
	var capturedRequest map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing auth header")
		}
		if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer ts.Close()

	logStore := llmlog.NewStore(10)
	client := NewClient(Config{
		BaseURL:  ts.URL,
		APIKey:   "test-key",
		Timeout:  3 * time.Second,
		LogStore: logStore,
	})

	resp, err := client.Chat(context.Background(), llm.ChatRequest{
		Purpose: "chat_reply",
		Model:   "mock-model",
		Messages: []llm.Message{
			{Role: "user", Content: "ping"},
		},
		Temperature: 0.1,
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.Content != "pong" {
		t.Fatalf("unexpected response content: %s", resp.Content)
	}

	if capturedRequest["model"] != "mock-model" {
		t.Fatalf("unexpected model: %v", capturedRequest["model"])
	}

	entries := logStore.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Purpose != "chat_reply" {
		t.Fatalf("unexpected purpose: %s", entries[0].Purpose)
	}
	if entries[0].StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", entries[0].StatusCode)
	}
	if entries[0].Request == "" || entries[0].Response == "" {
		t.Fatalf("request/response logs should not be empty")
	}
}
