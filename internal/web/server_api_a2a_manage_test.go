package web

import (
	"encoding/json"
	"fmt"
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

	saveBody := `{"name":"codex-local","description":"本地写代码助手","endpoint":"http://127.0.0.1:9091/a2a/rpc","enabled":true}`
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

func TestHandleAPIA2AAgentSave_DiscoversAgentCardAndSkills(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	s := &Server{mcpStore: store}

	baseURL := ""
	cardServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(fmt.Sprintf(`{
  "protocolVersion":"0.3.0",
  "name":"codex-local",
  "description":"local sdk agent",
  "url":"%s/a2a/rpc",
  "skills":[{"id":"codex_exec","name":"Codex Exec","description":"run codex","tags":["code"]}]
}`, baseURL)))
	}))
	defer cardServer.Close()
	baseURL = cardServer.URL

	saveBody := fmt.Sprintf(`{
  "agent_card_url":"%s/.well-known/agent-card.json",
  "enabled":true
}`, baseURL)
	saveReq := httptest.NewRequest(http.MethodPost, "/api/a2a/agents/save", strings.NewReader(saveBody))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	s.handleAPIA2AAgentSave(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("expected save 200, got %d body=%s", saveRec.Code, saveRec.Body.String())
	}

	agents := store.ListA2AAgents()
	if len(agents) != 1 {
		t.Fatalf("expected one agent, got %d", len(agents))
	}
	got := agents[0]
	if got.Endpoint != baseURL+"/a2a/rpc" {
		t.Fatalf("endpoint backfill mismatch: %q", got.Endpoint)
	}
	if got.Name != "codex-local" {
		t.Fatalf("name backfill mismatch: %q", got.Name)
	}
	if got.ProtocolVersion != "0.3.0" {
		t.Fatalf("protocol backfill mismatch: %q", got.ProtocolVersion)
	}
	if len(got.Skills) != 1 || got.Skills[0].ID != "codex_exec" {
		t.Fatalf("skills backfill mismatch: %+v", got.Skills)
	}
}

func TestHandleAPIA2AAgentSave_RejectsCardWithoutSkills(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	s := &Server{mcpStore: store}

	baseURL := ""
	cardServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(fmt.Sprintf(`{
  "protocolVersion":"0.3.0",
  "name":"bad-agent",
  "description":"missing skills",
  "url":"%s/a2a/rpc"
}`, baseURL)))
	}))
	defer cardServer.Close()
	baseURL = cardServer.URL

	saveBody := fmt.Sprintf(`{
  "agent_card_url":"%s/.well-known/agent-card.json",
  "enabled":true
}`, baseURL)
	saveReq := httptest.NewRequest(http.MethodPost, "/api/a2a/agents/save", strings.NewReader(saveBody))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	s.handleAPIA2AAgentSave(saveRec, saveReq)
	if saveRec.Code != http.StatusBadRequest {
		t.Fatalf("expected save 400, got %d body=%s", saveRec.Code, saveRec.Body.String())
	}
	if got := len(store.ListA2AAgents()); got != 0 {
		t.Fatalf("expected zero agents when skills missing, got %d", got)
	}
}

func TestHandleAPIA2AAgentSave_RejectsCardWithEmptySkills(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	s := &Server{mcpStore: store}

	baseURL := ""
	cardServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(fmt.Sprintf(`{
  "protocolVersion":"0.3.0",
  "name":"bad-agent",
  "description":"empty skills",
  "url":"%s/a2a/rpc",
  "skills":[]
}`, baseURL)))
	}))
	defer cardServer.Close()
	baseURL = cardServer.URL

	saveBody := fmt.Sprintf(`{
  "agent_card_url":"%s/.well-known/agent-card.json",
  "enabled":true
}`, baseURL)
	saveReq := httptest.NewRequest(http.MethodPost, "/api/a2a/agents/save", strings.NewReader(saveBody))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	s.handleAPIA2AAgentSave(saveRec, saveReq)
	if saveRec.Code != http.StatusBadRequest {
		t.Fatalf("expected save 400, got %d body=%s", saveRec.Code, saveRec.Body.String())
	}
	if got := len(store.ListA2AAgents()); got != 0 {
		t.Fatalf("expected zero agents when skills empty, got %d", got)
	}
}

func TestHandleAPIA2AAgentSave_RejectsUnreachableCard(t *testing.T) {
	store, err := mcp.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	s := &Server{mcpStore: store}

	saveBody := `{"agent_card_url":"http://127.0.0.1:1/.well-known/agent-card.json","enabled":true}`
	saveReq := httptest.NewRequest(http.MethodPost, "/api/a2a/agents/save", strings.NewReader(saveBody))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	s.handleAPIA2AAgentSave(saveRec, saveReq)
	if saveRec.Code != http.StatusBadRequest {
		t.Fatalf("expected save 400, got %d body=%s", saveRec.Code, saveRec.Body.String())
	}
	if got := len(store.ListA2AAgents()); got != 0 {
		t.Fatalf("expected zero agents when card unreachable, got %d", got)
	}
}
