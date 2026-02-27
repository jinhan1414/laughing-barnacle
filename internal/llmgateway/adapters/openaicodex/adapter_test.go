package openaicodex

import (
	"context"
	"encoding/json"
	"errors"
	"laughing-barnacle/internal/llmgateway"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAdapter_DefaultTransportAndNoForcedStore(t *testing.T) {
	authFile := writeAuthFile(t, "token-from-file")
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-from-file" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		_, _ = w.Write([]byte(`{"output_text":"pong","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`))
	}))
	defer server.Close()

	adapter := New(Config{
		BaseURL:      server.URL,
		AuthFilePath: authFile,
		Timeout:      2 * time.Second,
		Transport:    "",
	})
	resp, err := adapter.Chat(context.Background(), llmgateway.CanonicalChatRequest{
		Model:    "gpt-5-codex",
		Messages: []llmgateway.CanonicalMessage{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if requestBody["transport"] != "auto" {
		t.Fatalf("expected default transport=auto, got %v", requestBody["transport"])
	}
	if requestBody["store"] != false {
		t.Fatalf("expected codex default store=false, got %v", requestBody["store"])
	}
	if resp.Content != "pong" || resp.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestAdapter_RuntimeTransportAndStoreOverride(t *testing.T) {
	authFile := writeAuthFile(t, "token")
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"output_text":"ok"}`))
	}))
	defer server.Close()

	transport := "sse"
	store := false
	adapter := New(Config{
		BaseURL:      server.URL,
		AuthFilePath: authFile,
		Timeout:      2 * time.Second,
		Transport:    "auto",
	})
	_, err := adapter.Chat(context.Background(), llmgateway.CanonicalChatRequest{
		Model:    "gpt-5-codex",
		Messages: []llmgateway.CanonicalMessage{{Role: "user", Content: "ping"}},
		Options: llmgateway.CanonicalRequestOptions{
			Transport: &transport,
			Store:     &store,
		},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if requestBody["transport"] != "sse" {
		t.Fatalf("expected runtime transport override, got %v", requestBody["transport"])
	}
	if requestBody["store"] != false {
		t.Fatalf("expected runtime store override=false, got %v", requestBody["store"])
	}
}

func TestAdapter_ChatGPTAuthUsesCodexEndpointAndAccountHeader(t *testing.T) {
	authFile := writeChatGPTAuthFile(t, "chatgpt-access", "acct-123")
	var capturedPath string
	var capturedAccountID string
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAccountID = r.Header.Get("ChatGPT-Account-Id")
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"output_text":"ok"}`))
	}))
	defer server.Close()

	adapter := New(Config{
		BaseURL:      server.URL + "/backend-api",
		AuthFilePath: authFile,
		Timeout:      2 * time.Second,
	})
	_, err := adapter.Chat(context.Background(), llmgateway.CanonicalChatRequest{
		Purpose: "chat_reply",
		Model:   "gpt-5-codex",
		Messages: []llmgateway.CanonicalMessage{
			{Role: "system", Content: "You are a test assistant."},
			{Role: "user", Content: "ping"},
		},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if capturedPath != "/backend-api/codex/responses" {
		t.Fatalf("expected chatgpt codex path, got %q", capturedPath)
	}
	if capturedAccountID != "acct-123" {
		t.Fatalf("expected ChatGPT-Account-Id header, got %q", capturedAccountID)
	}
	if requestBody["instructions"] != "You are a test assistant." {
		t.Fatalf("expected instructions from system messages, got %v", requestBody["instructions"])
	}
	if _, ok := requestBody["prompt_cache_key"]; ok {
		t.Fatalf("expected first chat_reply request without prompt_cache_key, got %v", requestBody["prompt_cache_key"])
	}
}

func TestAdapter_ChatReplyReusesPromptCacheKeyFromPreviousResponse(t *testing.T) {
	authFile := writeChatGPTAuthFile(t, "chatgpt-access", "acct-123")
	var requestBodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requestBodies = append(requestBodies, requestBody)
		if len(requestBodies) == 1 {
			_, _ = w.Write([]byte(`{"output_text":"first","prompt_cache_key":"server-key-1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"output_text":"second"}`))
	}))
	defer server.Close()

	adapter := New(Config{
		BaseURL:      server.URL + "/backend-api",
		AuthFilePath: authFile,
		Timeout:      2 * time.Second,
	})
	req := llmgateway.CanonicalChatRequest{
		Purpose: "chat_reply",
		Model:   "gpt-5.3-codex",
		Messages: []llmgateway.CanonicalMessage{
			{Role: "user", Content: "你好"},
		},
	}
	if _, err := adapter.Chat(context.Background(), req); err != nil {
		t.Fatalf("first Chat error: %v", err)
	}
	if _, err := adapter.Chat(context.Background(), req); err != nil {
		t.Fatalf("second Chat error: %v", err)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requestBodies))
	}
	if _, ok := requestBodies[0]["prompt_cache_key"]; ok {
		t.Fatalf("expected first request without prompt_cache_key, got %v", requestBodies[0]["prompt_cache_key"])
	}
	if got, ok := requestBodies[1]["prompt_cache_key"].(string); !ok || got != "server-key-1" {
		t.Fatalf("expected second request prompt_cache_key=server-key-1, got %v", requestBodies[1]["prompt_cache_key"])
	}
}

func TestAdapter_ExplicitAuthFilePathHasHighestPriority(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("HOME", homeDir)

	defaultPath := filepath.Join(homeDir, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0o755); err != nil {
		t.Fatalf("mkdir default auth dir: %v", err)
	}
	if err := os.WriteFile(defaultPath, []byte(`{"access_token":"default-token"}`), 0o600); err != nil {
		t.Fatalf("write default auth file: %v", err)
	}
	explicitPath := writeAuthFile(t, "explicit-token")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer explicit-token" {
			t.Fatalf("explicit auth file should win, got %q", got)
		}
		_, _ = w.Write([]byte(`{"output_text":"ok"}`))
	}))
	defer server.Close()

	adapter := New(Config{
		BaseURL:      server.URL,
		APIToken:     "token-from-env",
		AuthFilePath: explicitPath,
		Timeout:      2 * time.Second,
	})
	_, err := adapter.Chat(context.Background(), llmgateway.CanonicalChatRequest{
		Model:    "gpt-5-codex",
		Messages: []llmgateway.CanonicalMessage{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
}

func TestAdapter_InvalidExplicitAuthFilePathReturnsExplicitError(t *testing.T) {
	adapter := New(Config{
		BaseURL:      "https://example.com",
		APIToken:     "token-from-env",
		AuthFilePath: filepath.Join(t.TempDir(), "missing.json"),
	})
	_, err := adapter.Chat(context.Background(), llmgateway.CanonicalChatRequest{
		Model:    "gpt-5-codex",
		Messages: []llmgateway.CanonicalMessage{{Role: "user", Content: "ping"}},
	})
	if err == nil {
		t.Fatalf("expected auth error")
	}
	var gatewayErr *llmgateway.Error
	if !errors.As(err, &gatewayErr) {
		t.Fatalf("expected gateway error, got %T", err)
	}
	if gatewayErr.Code != llmgateway.ErrorCodeAuthConfigInvalid {
		t.Fatalf("unexpected error code: %s", gatewayErr.Code)
	}
}

func TestAdapter_InvalidExplicitAuthFileFormatReturnsExplicitError(t *testing.T) {
	authFile := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(authFile, []byte(`{"unexpected":"field"}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	adapter := New(Config{
		BaseURL:      "https://example.com",
		AuthFilePath: authFile,
	})
	_, err := adapter.Chat(context.Background(), llmgateway.CanonicalChatRequest{
		Model:    "gpt-5-codex",
		Messages: []llmgateway.CanonicalMessage{{Role: "user", Content: "ping"}},
	})
	if err == nil {
		t.Fatalf("expected auth format error")
	}
	var gatewayErr *llmgateway.Error
	if !errors.As(err, &gatewayErr) {
		t.Fatalf("expected gateway error, got %T", err)
	}
	if gatewayErr.Code != llmgateway.ErrorCodeAuthConfigInvalid {
		t.Fatalf("unexpected error code: %s", gatewayErr.Code)
	}
}

func writeAuthFile(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.json")
	content := []byte(`{"access_token":"` + token + `"}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	return path
}

func writeChatGPTAuthFile(t *testing.T, token, accountID string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.json")
	content := []byte(
		`{"auth_mode":"chatgpt","tokens":{"access_token":"` + token +
			`","refresh_token":"r","account_id":"` + accountID + `"}}`,
	)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write chatgpt auth file: %v", err)
	}
	return path
}
