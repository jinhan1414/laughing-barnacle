package mcp

import (
	"path/filepath"
	"testing"
)

func TestStoreA2AAgentCRUDAndReload(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertA2AAgent(A2AAgent{
		Name:         "Codex Agent",
		Description:  "用于代码任务处理",
		Endpoint:     "http://127.0.0.1:9091/a2a",
		AgentCardURL: "http://127.0.0.1:9091/.well-known/agent-card.json",
		AuthToken:    "token-1",
		Enabled:      true,
	}); err != nil {
		t.Fatalf("UpsertA2AAgent create error: %v", err)
	}

	agents := store.ListA2AAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	agentID := agents[0].ID
	if agentID == "" {
		t.Fatalf("expected generated agent id")
	}

	if err := store.UpsertA2AAgent(A2AAgent{
		Name:     "Codex Agent Updated",
		Endpoint: "http://127.0.0.1:9091/a2a",
		Enabled:  false,
	}); err != nil {
		t.Fatalf("UpsertA2AAgent update by endpoint error: %v", err)
	}

	updated, ok := store.GetA2AAgent(agentID)
	if !ok {
		t.Fatalf("expected updated agent by id")
	}
	if updated.Name != "Codex Agent Updated" {
		t.Fatalf("unexpected updated name: %q", updated.Name)
	}
	if updated.AuthToken != "token-1" {
		t.Fatalf("expected auth token preserved, got %q", updated.AuthToken)
	}
	if updated.Enabled {
		t.Fatalf("expected updated agent disabled")
	}

	if err := store.SetA2AAgentEnabled(agentID, true); err != nil {
		t.Fatalf("SetA2AAgentEnabled error: %v", err)
	}
	if enabled := store.ListEnabledA2AAgents(); len(enabled) != 1 {
		t.Fatalf("expected one enabled a2a agent, got %d", len(enabled))
	}

	reloaded, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}
	reloadedAgent, ok := reloaded.GetA2AAgent(agentID)
	if !ok {
		t.Fatalf("expected agent after reload")
	}
	if !reloadedAgent.Enabled {
		t.Fatalf("expected enabled agent after reload")
	}
	if err := reloaded.DeleteA2AAgent(agentID); err != nil {
		t.Fatalf("DeleteA2AAgent error: %v", err)
	}
	if got := len(reloaded.ListA2AAgents()); got != 0 {
		t.Fatalf("expected no agents after delete, got %d", got)
	}
}

func TestStoreA2AAgentValidation(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	err = store.UpsertA2AAgent(A2AAgent{
		Name:     "bad",
		Endpoint: "localhost:9000",
		Enabled:  true,
	})
	if err == nil {
		t.Fatalf("expected endpoint validation error")
	}
}
