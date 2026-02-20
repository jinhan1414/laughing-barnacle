package cerber

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}`))
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
	if resp.Usage.TotalTokens != 15 || resp.Usage.PromptTokens != 12 || resp.Usage.CompletionTokens != 3 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
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
	if entries[0].Attempts != 1 {
		t.Fatalf("unexpected attempts: %d", entries[0].Attempts)
	}
	if entries[0].Request == "" || entries[0].Response == "" {
		t.Fatalf("request/response logs should not be empty")
	}
	if !strings.Contains(entries[0].Request, "\n") || !strings.Contains(entries[0].Response, "\n") {
		t.Fatalf("request/response logs should be pretty-printed JSON")
	}
}

func TestClientChat_RetryOn429ThenSuccess(t *testing.T) {
	var hits atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"rate limit"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok-after-retry"}}]}`))
	}))
	defer ts.Close()

	logStore := llmlog.NewStore(10)
	client := NewClient(Config{
		BaseURL:        ts.URL,
		APIKey:         "test-key",
		Timeout:        3 * time.Second,
		LogStore:       logStore,
		MaxRetries:     2,
		RetryBaseDelay: 1 * time.Millisecond,
		RetryMaxDelay:  5 * time.Millisecond,
	})

	resp, err := client.Chat(context.Background(), llm.ChatRequest{
		Purpose: "chat_reply",
		Model:   "mock-model",
		Messages: []llm.Message{
			{Role: "user", Content: "ping"},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.Content != "ok-after-retry" {
		t.Fatalf("unexpected response content: %s", resp.Content)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("expected 2 attempts, got %d", got)
	}

	entries := logStore.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Attempts != 2 {
		t.Fatalf("expected attempts=2, got %d", entries[0].Attempts)
	}
	if entries[0].StatusCode != http.StatusOK {
		t.Fatalf("expected final status=200, got %d", entries[0].StatusCode)
	}
}

func TestClientChat_RetryExhausted(t *testing.T) {
	var hits atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limit"}`))
	}))
	defer ts.Close()

	logStore := llmlog.NewStore(10)
	client := NewClient(Config{
		BaseURL:        ts.URL,
		APIKey:         "test-key",
		Timeout:        3 * time.Second,
		LogStore:       logStore,
		MaxRetries:     2,
		RetryBaseDelay: 1 * time.Millisecond,
		RetryMaxDelay:  5 * time.Millisecond,
	})

	_, err := client.Chat(context.Background(), llm.ChatRequest{
		Purpose: "chat_reply",
		Model:   "mock-model",
		Messages: []llm.Message{
			{Role: "user", Content: "ping"},
		},
	})
	if err == nil {
		t.Fatalf("expected rate-limit error")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("cerber status %d", http.StatusTooManyRequests)) {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}

	entries := logStore.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Attempts != 3 {
		t.Fatalf("expected attempts=3, got %d", entries[0].Attempts)
	}
	if entries[0].StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected status=429, got %d", entries[0].StatusCode)
	}
}
