package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/skills"
)

func TestHandleAPIMCPServiceSave_ConfigAndRedactedList(t *testing.T) {
	root := t.TempDir()
	mcpStore, err := mcp.NewStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore skills error: %v", err)
	}
	s := &Server{mcpStore: mcpStore, skillStore: skillStore}

	saveReq := httptest.NewRequest(http.MethodPost, "/api/mcp/services/save", strings.NewReader(`{"id":"http-demo","name":"http-demo","transport":"streamable_http","endpoint":"https://example.com/mcp","headers":{"X-Api-Key":"secret","X-Workspace":"prod"},"enabled":true}`))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	s.handleAPIMCPServiceSave(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", saveRec.Code, saveRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/mcp/services", nil)
	listRec := httptest.NewRecorder()
	s.handleAPIMCPServices(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var payload struct {
		Services []map[string]any `json:"services"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list payload: %v", err)
	}
	if len(payload.Services) != 1 {
		t.Fatalf("expected one service, got %+v", payload.Services)
	}
	service := payload.Services[0]
	if _, exists := service["headers"]; exists {
		t.Fatalf("expected redacted headers, got %+v", service)
	}
	if _, exists := service["env"]; exists {
		t.Fatalf("expected redacted env, got %+v", service)
	}
	if got := service["has_headers"]; got != true {
		t.Fatalf("expected has_headers=true, got %+v", service)
	}
	keys, _ := service["header_keys"].([]any)
	if len(keys) != 2 {
		t.Fatalf("expected two header keys, got %+v", service["header_keys"])
	}
}

func TestHandleAPIMCPServiceSave_ConfigValidation(t *testing.T) {
	root := t.TempDir()
	mcpStore, err := mcp.NewStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	skillStore, err := skills.NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore skills error: %v", err)
	}
	s := &Server{mcpStore: mcpStore, skillStore: skillStore}

	testCases := []struct {
		name          string
		body          string
		errorContains string
	}{
		{
			name:          "stdio rejects headers",
			body:          `{"name":"stdio-demo","transport":"stdio","command":"npx","headers":{"X-Test":"1"},"enabled":true}`,
			errorContains: "does not support headers",
		},
		{
			name:          "http rejects env",
			body:          `{"name":"http-demo","transport":"streamable_http","endpoint":"https://example.com/mcp","env":{"TOKEN":"1"},"enabled":true}`,
			errorContains: "does not support env",
		},
		{
			name:          "reject authorization header",
			body:          `{"name":"http-demo","transport":"sse","endpoint":"https://example.com/sse","headers":{"Authorization":"Bearer nope"},"enabled":true}`,
			errorContains: "Authorization",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/mcp/services/save", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			s.handleAPIMCPServiceSave(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(decodeAPIErrorMessage(t, rec), tc.errorContains) {
				t.Fatalf("unexpected error body=%s", rec.Body.String())
			}
		})
	}
}
