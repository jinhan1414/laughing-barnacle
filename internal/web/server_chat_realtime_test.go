package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

func TestHandleAPIChatSend_AcceptsBeforeAssistantReplyCompletes(t *testing.T) {
	server, store := newRealtimeTestServer(t, 200*time.Millisecond)
	req := httptest.NewRequest(http.MethodPost, "/api/chat/send", strings.NewReader(`{"message":"第一条消息"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	start := time.Now()
	server.handleAPIChatSend(rec, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if elapsed >= 150*time.Millisecond {
		t.Fatalf("expected accepted-first response, got latency %s", elapsed)
	}

	var payload apiChatSendResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v body=%s", err, rec.Body.String())
	}
	if !payload.OK || payload.MessageID == "" || payload.TurnID == "" || payload.AcceptedAtUS <= 0 {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	_, messages := store.Snapshot()
	if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
		t.Fatalf("assistant reply should not be ready at accept time: %+v", messages)
	}

	waitForAssistantReply(t, store, 2*time.Second)
}

func TestHandleAPIChatSend_QueuedTurnsRunSequentially(t *testing.T) {
	server, store := newRealtimeTestServer(t, 30*time.Millisecond)
	sendChatMessage(t, server, "第一条")
	sendChatMessage(t, server, "第二条")

	waitForMessages(t, store, 4, 3*time.Second)
	_, messages := store.Snapshot()
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}
	roles := []string{messages[0].Role, messages[1].Role, messages[2].Role, messages[3].Role}
	wantRoles := []string{"user", "assistant", "user", "assistant"}
	for i := range wantRoles {
		if roles[i] != wantRoles[i] {
			t.Fatalf("unexpected role order: got %+v want %+v", roles, wantRoles)
		}
	}
	if messages[0].Content != "第一条" || messages[2].Content != "第二条" {
		t.Fatalf("unexpected message ordering: %+v", messages)
	}
}

func TestHandleAPIChatStream_EmitsAssistantAndTurnStatusEvents(t *testing.T) {
	store := conversation.NewStore()
	store.AppendEvent(chatTurnEventType, "turn_id=turn_1 | message_id=msg_1 | status=queued | brief=测试")
	store.AppendAssistant("已完成", nil)

	server := &Server{convStore: store}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream?since_us=0", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		server.handleAPIChatStream(rec, req)
		close(done)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "event: turn.status") {
		t.Fatalf("expected turn.status event, got body=%s", body)
	}
	if !strings.Contains(body, "event: assistant.message") {
		t.Fatalf("expected assistant.message event, got body=%s", body)
	}
}

func TestHandleAPIChatUpdates_IncludesTurnStatusFallback(t *testing.T) {
	store := conversation.NewStore()
	store.AppendEvent(chatTurnEventType, "turn_id=turn_1 | message_id=msg_1 | status=working | brief=测试")
	server := &Server{convStore: store}

	req := httptest.NewRequest(http.MethodGet, "/api/chat/updates?since_us=0", nil)
	rec := httptest.NewRecorder()
	server.handleAPIChatUpdates(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Updates []apiChatUpdate `json:"updates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v body=%s", err, rec.Body.String())
	}
	if len(payload.Updates) != 1 || payload.Updates[0].EventType != chatTurnEventType {
		t.Fatalf("unexpected updates: %+v", payload.Updates)
	}
}

func TestHandleAPIChatUpdates_ReturnsMultipleAsyncTaskEvents(t *testing.T) {
	store := conversation.NewStore()
	store.AppendEvent("async_task_status", "task_id=task_1 | type=a2a | status=working")
	store.AppendEvent("async_task_status", "task_id=task_2 | type=a2a | status=working")
	server := &Server{convStore: store}

	req := httptest.NewRequest(http.MethodGet, "/api/chat/updates?since_us=0", nil)
	rec := httptest.NewRecorder()
	server.handleAPIChatUpdates(rec, req)

	var payload struct {
		Updates []apiChatUpdate `json:"updates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v body=%s", err, rec.Body.String())
	}
	if len(payload.Updates) != 2 {
		t.Fatalf("expected two async task events, got %+v", payload.Updates)
	}
}

func TestHandleAPIChatStream_ResumesFromCursor(t *testing.T) {
	store := conversation.NewStore()
	store.AppendEvent(chatTurnEventType, "turn_id=turn_1 | message_id=msg_1 | status=queued | brief=第一条")
	time.Sleep(2 * time.Millisecond)
	store.AppendEvent(chatTurnEventType, "turn_id=turn_2 | message_id=msg_2 | status=queued | brief=第二条")

	server := &Server{convStore: store}
	_, _, events := store.SnapshotWithEvents()
	firstCursor := events[0].CreatedAt.UnixMicro()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream?since_us="+strconv.FormatInt(firstCursor, 10), nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		server.handleAPIChatStream(rec, req)
		close(done)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if strings.Contains(body, "turn_id=turn_1") {
		t.Fatalf("expected stream to resume after cursor, got body=%s", body)
	}
	if !strings.Contains(body, "turn_id=turn_2") {
		t.Fatalf("expected stream to include later event, got body=%s", body)
	}
}

type realtimeTestLLM struct {
	mu        sync.Mutex
	delay     time.Duration
	callCount int
}

func (m *realtimeTestLLM) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	if m.delay > 0 {
		timer := time.NewTimer(m.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return llm.ChatResponse{}, ctx.Err()
		case <-timer.C:
		}
	}
	m.mu.Lock()
	m.callCount++
	count := m.callCount
	m.mu.Unlock()
	return llm.ChatResponse{Content: "reply-" + strconv.Itoa(count)}, nil
}

func newRealtimeTestServer(t *testing.T, delay time.Duration) (*Server, *conversation.Store) {
	t.Helper()
	store := conversation.NewStore()
	agentSvc := agent.New(agent.Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          1,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compress",
	}, store, &realtimeTestLLM{delay: delay}, nil)
	server, err := NewServer(agentSvc, store, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	return server, store
}

func sendChatMessage(t *testing.T, server *Server, message string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/chat/send", strings.NewReader(`{"message":"`+message+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.handleAPIChatSend(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func waitForAssistantReply(t *testing.T, store *conversation.Store, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, messages := store.Snapshot()
		if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, messages := store.Snapshot()
	t.Fatalf("assistant reply not found before timeout, messages=%+v", messages)
}

func waitForMessages(t *testing.T, store *conversation.Store, count int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, messages := store.Snapshot()
		if len(messages) >= count {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, messages := store.Snapshot()
	t.Fatalf("messages did not reach %d before timeout, got %+v", count, messages)
}
