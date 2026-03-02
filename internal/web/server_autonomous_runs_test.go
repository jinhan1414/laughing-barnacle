package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
)

func TestHandleAPIChatUpdates_IncludesAutonomousRunStatus(t *testing.T) {
	store := conversation.NewStore()
	store.AppendEvent("autonomous_run_status", "run_id=run_1 | status=waiting_async | step=generate_topics | goal=自动运营")
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
	if len(payload.Updates) != 1 || payload.Updates[0].EventType != "autonomous_run_status" {
		t.Fatalf("unexpected updates: %+v", payload.Updates)
	}
}

func TestHandleSettingsPage_RendersAutonomousRunsSection(t *testing.T) {
	store, err := conversation.NewStoreWithFile(filepath.Join(t.TempDir(), "conversation.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
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
	}, store, nil, nil)
	seed := []agent.AutonomousRun{
		{
			ID:          "run_1",
			Goal:        "自动运营小红书 7 天",
			Status:      "waiting_async",
			CurrentStep: "generate_topics",
			WaitingType: "async_task",
			WaitingRef:  "async_1",
			CreatedAt:   time.Date(2026, 3, 2, 8, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 3, 2, 8, 5, 0, 0, time.UTC),
			Steps: []agent.RunStep{
				{
					Seq:       1,
					Step:      "generate_topics",
					Status:    "waiting_async",
					Summary:   "已提交选题生成任务",
					CreatedAt: time.Date(2026, 3, 2, 8, 1, 0, 0, time.UTC),
				},
			},
		},
	}
	raw, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("marshal run seed error: %v", err)
	}
	if err := store.SaveAutonomousRunState(raw); err != nil {
		t.Fatalf("SaveAutonomousRunState error: %v", err)
	}
	if err := agentSvc.BindAutonomousRunStateStore(agent.NewConversationAutonomousRunStateStore(store)); err != nil {
		t.Fatalf("BindAutonomousRunStateStore error: %v", err)
	}

	server, err := NewServer(agentSvc, store, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/settings?section=autonomous_runs", nil)
	rec := httptest.NewRecorder()
	server.handleSettingsPage(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, body)
	}
	if !strings.Contains(body, "自主运行") || !strings.Contains(body, "自动运营小红书 7 天") || !strings.Contains(body, "generate_topics") {
		t.Fatalf("expected autonomous run section rendered, got body=%s", body)
	}
}
