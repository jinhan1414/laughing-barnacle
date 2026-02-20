package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"laughing-barnacle/internal/conversation"
)

func TestHandleAPIChatUpdates_ReturnsIncrementalAssistantAndEvents(t *testing.T) {
	convStore := conversation.NewStore()
	convStore.Append("user", "u1")
	time.Sleep(2 * time.Millisecond)
	convStore.AppendAssistant("a1", &conversation.TokenUsage{
		PromptTokens:     50,
		CompletionTokens: 10,
		TotalTokens:      60,
	})
	time.Sleep(2 * time.Millisecond)
	convStore.AppendEvent("context_compression", "c1")

	s := &Server{convStore: convStore}
	req := httptest.NewRequest(http.MethodGet, "/api/chat/updates?since_us=0", nil)
	rec := httptest.NewRecorder()
	s.handleAPIChatUpdates(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Updates []apiChatUpdate `json:"updates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v body=%s", err, rec.Body.String())
	}
	if len(payload.Updates) != 2 {
		t.Fatalf("expected 2 updates (assistant + event), got %d", len(payload.Updates))
	}
	if payload.Updates[0].Kind != "assistant" || payload.Updates[0].Content != "a1" {
		t.Fatalf("unexpected first update: %+v", payload.Updates[0])
	}
	if payload.Updates[0].Usage == nil || payload.Updates[0].Usage.TotalTokens != 60 {
		t.Fatalf("unexpected usage in first update: %+v", payload.Updates[0].Usage)
	}
	if payload.Updates[1].Kind != "event" || payload.Updates[1].Content != "c1" {
		t.Fatalf("unexpected second update: %+v", payload.Updates[1])
	}
	if payload.Updates[0].CreatedAtUS <= 0 || payload.Updates[1].CreatedAtUS <= payload.Updates[0].CreatedAtUS {
		t.Fatalf("unexpected timestamps: %+v", payload.Updates)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/chat/updates?since_us="+strconv.FormatInt(payload.Updates[0].CreatedAtUS, 10), nil)
	rec2 := httptest.NewRecorder()
	s.handleAPIChatUpdates(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 on incremental query, got %d body=%s", rec2.Code, rec2.Body.String())
	}

	var payload2 struct {
		Updates []apiChatUpdate `json:"updates"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &payload2); err != nil {
		t.Fatalf("decode second response error: %v body=%s", err, rec2.Body.String())
	}
	if len(payload2.Updates) != 1 || payload2.Updates[0].Kind != "event" || payload2.Updates[0].Content != "c1" {
		t.Fatalf("unexpected incremental updates: %+v", payload2.Updates)
	}
}

func TestHandleAPIChatUpdates_InvalidCursor(t *testing.T) {
	s := &Server{convStore: conversation.NewStore()}
	req := httptest.NewRequest(http.MethodGet, "/api/chat/updates?since_us=bad", nil)
	rec := httptest.NewRecorder()
	s.handleAPIChatUpdates(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}
