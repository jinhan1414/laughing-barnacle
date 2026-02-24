package mcp

import (
	"path/filepath"
	"testing"
)

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
		Name:    "Writing Mode",
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

func TestStoreUpsertSkill_EmptyIDUpdatesExistingByName(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		ID:      "research",
		Name:    "Research Mode",
		Prompt:  "先检索",
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertSkill error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		Name:    "Research Mode",
		Prompt:  "先检索再回答并附来源",
		Enabled: false,
	}); err != nil {
		t.Fatalf("UpsertSkill update error: %v", err)
	}

	skills := store.ListSkills()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill after update, got %d", len(skills))
	}
	if skills[0].ID != "research" {
		t.Fatalf("expected id keep research, got %q", skills[0].ID)
	}
	if skills[0].Prompt != "先检索再回答并附来源" {
		t.Fatalf("expected prompt updated, got %q", skills[0].Prompt)
	}
	if skills[0].Enabled {
		t.Fatalf("expected skill disabled after update")
	}
}

func TestStoreUpsertService_EmptyIDUpdatesExistingByEndpoint(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertService(Service{
		ID:        "deepwiki",
		Name:      "DeepWiki",
		Endpoint:  "https://mcp.deepwiki.com/mcp",
		Transport: "streamable_http",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService error: %v", err)
	}

	if err := store.UpsertService(Service{
		Name:      "DeepWiki PROD",
		Endpoint:  "https://mcp.deepwiki.com/mcp",
		Transport: "streamableHttp",
		Enabled:   false,
	}); err != nil {
		t.Fatalf("UpsertService update error: %v", err)
	}

	services := store.ListServices()
	if len(services) != 1 {
		t.Fatalf("expected 1 service after update, got %d", len(services))
	}
	if services[0].ID != "deepwiki" {
		t.Fatalf("expected id keep deepwiki, got %q", services[0].ID)
	}
	if services[0].Name != "DeepWiki PROD" {
		t.Fatalf("expected name updated, got %q", services[0].Name)
	}
	if services[0].Enabled {
		t.Fatalf("expected service disabled after update")
	}
	if services[0].Transport != ServiceTransportStreamableHTTP {
		t.Fatalf("expected normalized streamable transport, got %q", services[0].Transport)
	}
}

func TestStoreSetServiceToolEnabled_Persisted(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertService(Service{
		ID:        "search",
		Name:      "Search",
		Endpoint:  "https://example.com/mcp",
		Transport: "streamable_http",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService error: %v", err)
	}

	if !store.IsServiceToolEnabled("search", "web_search") {
		t.Fatalf("tool should be enabled by default")
	}
	if err := store.SetServiceToolEnabled("search", "web_search", false); err != nil {
		t.Fatalf("SetServiceToolEnabled disable error: %v", err)
	}
	if store.IsServiceToolEnabled("search", "web_search") {
		t.Fatalf("tool should be disabled")
	}

	if err := store.SetServiceToolEnabled("search", "web_search", true); err != nil {
		t.Fatalf("SetServiceToolEnabled enable error: %v", err)
	}
	if !store.IsServiceToolEnabled("search", "web_search") {
		t.Fatalf("tool should be enabled after toggle back")
	}

	if err := store.SetServiceToolEnabled("search", "weather", false); err != nil {
		t.Fatalf("SetServiceToolEnabled second tool disable error: %v", err)
	}

	reloaded, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}
	if !reloaded.IsServiceToolEnabled("search", "web_search") {
		t.Fatalf("web_search should stay enabled after reload")
	}
	if reloaded.IsServiceToolEnabled("search", "weather") {
		t.Fatalf("weather should stay disabled after reload")
	}
}
