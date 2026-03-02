package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreUpsertService_PreservesAndClearsScopedConfig(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertService(Service{
		ID:        "stdio-demo",
		Name:      "stdio-demo",
		Transport: "stdio",
		Command:   "npx",
		Env:       map[string]string{"TAVILY_API_KEY": "token-1"},
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService stdio error: %v", err)
	}
	if err := store.UpsertService(Service{
		ID:        "stdio-demo",
		Name:      "stdio-demo",
		Transport: "stdio",
		Command:   "npx",
		Enabled:   false,
	}); err != nil {
		t.Fatalf("UpsertService preserve env error: %v", err)
	}
	stdioService, ok := store.GetService("stdio-demo")
	if !ok {
		t.Fatalf("expected stdio service")
	}
	if got := stdioService.Env["TAVILY_API_KEY"]; got != "token-1" {
		t.Fatalf("expected preserved env value, got %q", got)
	}

	if err := store.UpsertService(Service{
		ID:        "stdio-demo",
		Name:      "stdio-demo",
		Transport: "stdio",
		Command:   "npx",
		Env:       map[string]string{},
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService clear env error: %v", err)
	}
	stdioService, ok = store.GetService("stdio-demo")
	if !ok {
		t.Fatalf("expected stdio service after clear")
	}
	if len(stdioService.Env) != 0 {
		t.Fatalf("expected cleared env, got %+v", stdioService.Env)
	}

	if err := store.UpsertService(Service{
		ID:        "http-demo",
		Name:      "http-demo",
		Transport: "streamable_http",
		Endpoint:  "https://example.com/mcp",
		Headers:   map[string]string{"X-Api-Key": "key-1"},
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService http error: %v", err)
	}
	if err := store.UpsertService(Service{
		ID:        "http-demo",
		Name:      "http-demo",
		Transport: "streamable_http",
		Endpoint:  "https://example.com/mcp",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService preserve headers error: %v", err)
	}
	httpService, ok := store.GetService("http-demo")
	if !ok {
		t.Fatalf("expected http service")
	}
	if got := httpService.Headers["X-Api-Key"]; got != "key-1" {
		t.Fatalf("expected preserved header value, got %q", got)
	}

	if err := store.UpsertService(Service{
		ID:        "http-demo",
		Name:      "http-demo",
		Transport: "streamable_http",
		Endpoint:  "https://example.com/mcp",
		Headers:   map[string]string{},
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService clear headers error: %v", err)
	}
	httpService, ok = store.GetService("http-demo")
	if !ok {
		t.Fatalf("expected http service after clear")
	}
	if len(httpService.Headers) != 0 {
		t.Fatalf("expected cleared headers, got %+v", httpService.Headers)
	}
}

func TestStoreUpsertService_RejectsInvalidScopedConfig(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	testCases := []struct {
		name          string
		service       Service
		errorContains string
	}{
		{
			name: "http rejects env",
			service: Service{
				Name:      "http-demo",
				Transport: "streamable_http",
				Endpoint:  "https://example.com/mcp",
				Env:       map[string]string{"TOKEN": "x"},
				Enabled:   true,
			},
			errorContains: "does not support env",
		},
		{
			name: "stdio rejects headers",
			service: Service{
				Name:      "stdio-demo",
				Transport: "stdio",
				Command:   "npx",
				Headers:   map[string]string{"X-Test": "x"},
				Enabled:   true,
			},
			errorContains: "does not support headers",
		},
		{
			name: "reject authorization header",
			service: Service{
				Name:      "http-demo",
				Transport: "sse",
				Endpoint:  "https://example.com/sse",
				Headers:   map[string]string{"Authorization": "Bearer bad"},
				Enabled:   true,
			},
			errorContains: authorizationHeaderName,
		},
		{
			name: "reject empty env value",
			service: Service{
				Name:      "stdio-demo",
				Transport: "stdio",
				Command:   "npx",
				Env:       map[string]string{"TOKEN": "   "},
				Enabled:   true,
			},
			errorContains: "must be non-empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.UpsertService(tc.service)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.errorContains) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
