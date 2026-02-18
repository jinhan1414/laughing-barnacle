package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llmlog"
)

func TestHandleLogsPage_RendersCopyButtonsPerEntry(t *testing.T) {
	logStore := llmlog.NewStore(10)
	logStore.Add(llmlog.Entry{
		Purpose:    "chat_reply",
		Model:      "test-model",
		StatusCode: 200,
		DurationMS: 12,
		Request:    `{"foo":"bar"}`,
		Response:   `{"ok":true}`,
	})

	server, err := NewServer(nil, conversation.NewStore(), logStore, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	req := httptest.NewRequest("GET", "/logs", nil)
	rr := httptest.NewRecorder()
	server.handleLogsPage(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "复制请求") {
		t.Fatalf("expected copy-request button, got body=%q", body)
	}
	if !strings.Contains(body, "复制响应") {
		t.Fatalf("expected copy-response button, got body=%q", body)
	}
	if !strings.Contains(body, "复制本次请求+响应") {
		t.Fatalf("expected copy-all button, got body=%q", body)
	}
}
