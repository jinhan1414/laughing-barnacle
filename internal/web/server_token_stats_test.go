package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llmlog"
)

func TestHandleSettingsPage_RendersTokenStatsSection(t *testing.T) {
	logStore := llmlog.NewStore(10)
	logStore.Add(llmlog.Entry{
		Provider:         "openai-codex",
		Model:            "openai-codex/gpt-5-codex",
		PromptTokens:     2000,
		CompletionTokens: 500,
		TotalTokens:      2500,
		CachedTokens:     1000,
	})

	server, err := NewServer(nil, conversation.NewStore(), logStore, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings?section=token_stats", nil)
	rec := httptest.NewRecorder()
	server.handleSettingsPage(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, body)
	}
	if !strings.Contains(body, "Token 消耗统计") {
		t.Fatalf("expected token stats section title, got body=%s", body)
	}
	if !strings.Contains(body, "openai-codex") || !strings.Contains(body, "2.5K") {
		t.Fatalf("expected channel stats rendered, got body=%s", body)
	}
}
