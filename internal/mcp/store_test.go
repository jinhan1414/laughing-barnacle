package mcp

import (
	"path/filepath"
	"testing"
)

func TestStoreUpsertAndReload(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertService(Service{
		ID:        "search",
		Name:      "Search",
		Endpoint:  "https://example.com/mcp",
		AuthToken: "token-1",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService error: %v", err)
	}

	if err := store.UpsertService(Service{
		ID:       "search",
		Name:     "Search API",
		Endpoint: "https://example.com/mcp",
		Enabled:  false,
	}); err != nil {
		t.Fatalf("UpsertService update error: %v", err)
	}

	svc, ok := store.GetService("search")
	if !ok {
		t.Fatalf("service not found")
	}
	if svc.AuthToken != "token-1" {
		t.Fatalf("expected auth token to be preserved, got %q", svc.AuthToken)
	}
	if svc.Enabled {
		t.Fatalf("expected service disabled")
	}

	reloaded, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}

	services := reloaded.ListServices()
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	if services[0].Name != "Search API" {
		t.Fatalf("unexpected service name: %q", services[0].Name)
	}
	if services[0].AuthToken != "token-1" {
		t.Fatalf("unexpected token after reload: %q", services[0].AuthToken)
	}
}

func TestStoreSkillCRUDAndPrompts(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		ID:      "research",
		Name:    "Research Skill",
		Prompt:  "先检索再回答，给出来源。",
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertSkill error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		ID:      "research",
		Name:    "Research Skill v2",
		Prompt:  "先检索、再总结、最后给出来源。",
		Enabled: false,
	}); err != nil {
		t.Fatalf("UpsertSkill update error: %v", err)
	}

	if err := store.SetSkillEnabled("research", true); err != nil {
		t.Fatalf("SetSkillEnabled error: %v", err)
	}

	prompts := store.ListEnabledSkillPrompts()
	if len(prompts) != 1 {
		t.Fatalf("expected 1 enabled prompt, got %d", len(prompts))
	}
	if prompts[0] != "先检索、再总结、最后给出来源。" {
		t.Fatalf("unexpected prompt: %q", prompts[0])
	}

	if err := store.DeleteSkill("research"); err != nil {
		t.Fatalf("DeleteSkill error: %v", err)
	}
	if len(store.ListSkills()) != 0 {
		t.Fatalf("expected no skills after delete")
	}
}

func TestStoreUpsertService_AutoGeneratesIDWhenMissing(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertService(Service{
		Name:     "Search MCP",
		Endpoint: "https://example.com/mcp/search",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertService error: %v", err)
	}
	if err := store.UpsertService(Service{
		Name:     "Search MCP",
		Endpoint: "https://example.com/mcp/search2",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertService second insert error: %v", err)
	}

	services := store.ListServices()
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
	if services[0].ID == "" || services[1].ID == "" {
		t.Fatalf("expected generated ids, got %+v", services)
	}
	if services[0].ID == services[1].ID {
		t.Fatalf("expected unique generated ids, got %q", services[0].ID)
	}
}

func TestStoreUpsertSkill_AutoGeneratesIDWhenMissing(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		Name:    "Research Mode",
		Prompt:  "先检索再回答",
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertSkill error: %v", err)
	}
	if err := store.UpsertSkill(Skill{
		Name:    "Research Mode",
		Prompt:  "先检索再回答，附来源",
		Enabled: false,
	}); err != nil {
		t.Fatalf("UpsertSkill second insert error: %v", err)
	}

	skills := store.ListSkills()
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}
	if skills[0].ID == "" || skills[1].ID == "" {
		t.Fatalf("expected generated ids, got %+v", skills)
	}
	if skills[0].ID == skills[1].ID {
		t.Fatalf("expected unique generated ids, got %q", skills[0].ID)
	}
}
