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
