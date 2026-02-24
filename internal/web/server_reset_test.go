package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llmlog"
)

func TestHandleChatReset_ClearsConversationAndLogs(t *testing.T) {
	convPath := filepath.Join(t.TempDir(), "conversation.json")
	logPath := filepath.Join(t.TempDir(), "llm_logs.json")

	convStore, err := conversation.NewStoreWithFile(convPath)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = convStore.Close() })
	logStore, err := llmlog.NewStoreWithFile(10, logPath)
	if err != nil {
		t.Fatalf("NewStoreWithFile llmlog error: %v", err)
	}

	convStore.Append("user", "hello")
	convStore.Append("assistant", "world")
	convStore.SetSummaryAndTrim("summary", 10)
	convStore.AppendEvent("context_compression", "compressed")
	logStore.Add(llmlog.Entry{Purpose: "chat_reply", Request: "req"})

	s := &Server{
		convStore: convStore,
		logStore:  logStore,
	}

	req := httptest.NewRequest(http.MethodPost, "/chat/reset", nil)
	rec := httptest.NewRecorder()
	s.handleChatReset(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected status 302, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/chat" {
		t.Fatalf("expected redirect to /chat, got %q", got)
	}

	summary, messages, events := convStore.SnapshotWithEvents()
	if summary != "" || len(messages) != 0 || len(events) != 0 {
		t.Fatalf("expected empty conversation after reset, got summary=%q messages=%d events=%d", summary, len(messages), len(events))
	}
	if got := len(logStore.List()); got != 0 {
		t.Fatalf("expected empty llm logs after reset, got %d", got)
	}
}

func TestHandleChatReset_MethodNotAllowed(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/chat/reset", nil)
	rec := httptest.NewRecorder()
	s.handleChatReset(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}
