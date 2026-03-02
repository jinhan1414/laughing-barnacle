package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llmlog"
	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/memory"
)

func TestHandleChatReset_ClearsConversationAndLogs(t *testing.T) {
	convPath := filepath.Join(t.TempDir(), "conversation.json")
	logPath := filepath.Join(t.TempDir(), "llm_logs.json")
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	memoryPath := filepath.Join(t.TempDir(), "memory.db")

	convStore, err := conversation.NewStoreWithFile(convPath)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = convStore.Close() })
	logStore, err := llmlog.NewStoreWithFile(10, logPath)
	if err != nil {
		t.Fatalf("NewStoreWithFile llmlog error: %v", err)
	}
	mcpStore, err := mcp.NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore mcp error: %v", err)
	}
	memoryStore, err := memory.NewStoreWithFile(memoryPath)
	if err != nil {
		t.Fatalf("NewStoreWithFile memory error: %v", err)
	}
	t.Cleanup(func() { _ = memoryStore.Close() })

	convStore.Append("user", "hello")
	convStore.Append("assistant", "world")
	convStore.SetSummaryAndTrim("summary", 10)
	convStore.AppendEvent("context_compression", "compressed")
	logStore.Add(llmlog.Entry{Purpose: "chat_reply", Request: "req"})
	if _, err := memoryStore.UpsertNode(memory.UpsertRequest{
		Mode:          "replace",
		Path:          "/projects/reset-demo/overview",
		Type:          memory.NodeTypeFile,
		Title:         "概览",
		SchemaKind:    "project_overview",
		SchemaVersion: 1,
		Summary:       "待清空",
	}); err != nil {
		t.Fatalf("memory UpsertNode error: %v", err)
	}
	if err := mcpStore.SetLastChatGreetingState("2026-02-26", time.Date(2026, 2, 26, 8, 0, 0, 0, time.UTC), "早上好"); err != nil {
		t.Fatalf("SetLastChatGreetingState error: %v", err)
	}
	seedTasks := []agent.AsyncTask{
		{
			ID:        "async_20260228_120000_1",
			TaskType:  "generic",
			Status:    "working",
			Request:   "后台任务测试",
			CreatedAt: time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC),
		},
	}
	rawTasks, err := json.Marshal(seedTasks)
	if err != nil {
		t.Fatalf("marshal async tasks error: %v", err)
	}
	if err := convStore.SaveAsyncTaskState(rawTasks); err != nil {
		t.Fatalf("SaveAsyncTaskState error: %v", err)
	}
	seedRuns := []agent.AutonomousRun{
		{
			ID:          "run_20260302_1",
			Goal:        "自动运营小红书 7 天",
			Status:      "waiting_async",
			CurrentStep: "generate_topics",
			WaitingType: "async_task",
			WaitingRef:  "async_20260228_120000_1",
			CreatedAt:   time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC),
		},
	}
	rawRuns, err := json.Marshal(seedRuns)
	if err != nil {
		t.Fatalf("marshal autonomous runs error: %v", err)
	}
	if err := convStore.SaveAutonomousRunState(rawRuns); err != nil {
		t.Fatalf("SaveAutonomousRunState error: %v", err)
	}
	agentSvc := agent.New(agent.Config{}, convStore, nil, nil)
	if err := agentSvc.BindAsyncTaskStateStore(agent.NewConversationAsyncTaskStateStore(convStore)); err != nil {
		t.Fatalf("BindAsyncTaskStateStore error: %v", err)
	}
	if err := agentSvc.BindAutonomousRunStateStore(agent.NewConversationAutonomousRunStateStore(convStore)); err != nil {
		t.Fatalf("BindAutonomousRunStateStore error: %v", err)
	}
	if got := len(agentSvc.ListAsyncTasks()); got != 1 {
		t.Fatalf("expected seeded async task in memory, got %d", got)
	}
	if got := len(agentSvc.ListAutonomousRuns()); got != 1 {
		t.Fatalf("expected seeded autonomous run in memory, got %d", got)
	}

	s := &Server{
		agent:       agentSvc,
		convStore:   convStore,
		logStore:    logStore,
		mcpStore:    mcpStore,
		memoryStore: memoryStore,
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
	if _, err := memoryStore.ReadNode("/projects/reset-demo/overview"); err == nil {
		t.Fatalf("expected memory file removed after reset")
	}
	if _, err := memoryStore.ReadNode("/projects"); err != nil {
		t.Fatalf("expected default memory namespace restored, got error: %v", err)
	}
	if got := mcpStore.GetLastChatGreetingDate(); got != "" {
		t.Fatalf("expected chat greeting date cleared, got %q", got)
	}
	if got := mcpStore.GetLastChatGreetingAt(); !got.IsZero() {
		t.Fatalf("expected chat greeting time cleared, got %s", got.Format(time.RFC3339))
	}
	if got := mcpStore.GetLastChatGreetingContent(); got != "" {
		t.Fatalf("expected chat greeting content cleared, got %q", got)
	}
	if got := len(agentSvc.ListAsyncTasks()); got != 0 {
		t.Fatalf("expected empty async tasks after reset, got %d", got)
	}
	if got := len(agentSvc.ListAutonomousRuns()); got != 0 {
		t.Fatalf("expected empty autonomous runs after reset, got %d", got)
	}
	rawTasks, err = convStore.LoadAsyncTaskState()
	if err != nil {
		t.Fatalf("LoadAsyncTaskState error: %v", err)
	}
	if len(rawTasks) != 0 {
		t.Fatalf("expected persisted async task state cleared, got %q", string(rawTasks))
	}
	rawRuns, err = convStore.LoadAutonomousRunState()
	if err != nil {
		t.Fatalf("LoadAutonomousRunState error: %v", err)
	}
	if len(rawRuns) != 0 {
		t.Fatalf("expected persisted autonomous run state cleared, got %q", string(rawRuns))
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
