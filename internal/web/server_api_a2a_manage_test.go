package web

import (
	"encoding/json"
	"laughing-barnacle/internal/mcp"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleAPIA2AAgentManage_SaveToggleDelete(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	s := &Server{mcpStore: store}

	saveBody := `{"name":"codex-local","description":"本地写代码助手","endpoint":"http://127.0.0.1:9091/a2a/rpc","agent_card_url":"http://127.0.0.1:9091/.well-known/agent-card.json","enabled":true}`
	saveReq := httptest.NewRequest(http.MethodPost, "/api/a2a/agents/save", strings.NewReader(saveBody))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	s.handleAPIA2AAgentSave(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("expected save 200, got %d body=%s", saveRec.Code, saveRec.Body.String())
	}

	agents := store.ListA2AAgents()
	if len(agents) != 1 {
		t.Fatalf("expected one agent after save, got %d", len(agents))
	}
	agentID := strings.TrimSpace(agents[0].ID)
	if agentID == "" {
		t.Fatalf("expected generated agent id")
	}
	if !agents[0].Enabled {
		t.Fatalf("expected enabled=true after save")
	}

	toggleBody := `{"id":"` + agentID + `","enabled":false}`
	toggleReq := httptest.NewRequest(http.MethodPost, "/api/a2a/agents/toggle", strings.NewReader(toggleBody))
	toggleReq.Header.Set("Content-Type", "application/json")
	toggleRec := httptest.NewRecorder()
	s.handleAPIA2AAgentToggle(toggleRec, toggleReq)
	if toggleRec.Code != http.StatusOK {
		t.Fatalf("expected toggle 200, got %d body=%s", toggleRec.Code, toggleRec.Body.String())
	}
	toggled, ok := store.GetA2AAgent(agentID)
	if !ok {
		t.Fatalf("expected toggled agent exists")
	}
	if toggled.Enabled {
		t.Fatalf("expected enabled=false after toggle")
	}

	deleteBody := `{"id":"` + agentID + `"}`
	deleteReq := httptest.NewRequest(http.MethodPost, "/api/a2a/agents/delete", strings.NewReader(deleteBody))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteRec := httptest.NewRecorder()
	s.handleAPIA2AAgentDelete(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	if got := len(store.ListA2AAgents()); got != 0 {
		t.Fatalf("expected no agents after delete, got %d", got)
	}
}

func TestHandleAPIA2AAgentSave_RejectsInvalidJSON(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	s := &Server{mcpStore: store}

	req := httptest.NewRequest(http.MethodPost, "/api/a2a/agents/save", strings.NewReader(`{"name":"codex-local"`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleAPIA2AAgentSave(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response error: %v body=%s", err, rec.Body.String())
	}
	errMsg, _ := payload["error"].(string)
	if strings.TrimSpace(errMsg) == "" {
		t.Fatalf("expected non-empty error message")
	}
}
