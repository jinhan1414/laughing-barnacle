package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"laughing-barnacle/internal/conversation"
)

func TestCallContextRead_ListSchedules(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/schedules" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"schedules":[]}`))
	}))
	t.Cleanup(apiServer.Close)

	agentSvc := New(Config{
		Model:                      "test-model",
		LocalAPIBaseURL:            apiServer.URL,
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          2,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, conversation.NewStore(), &mockLLM{}, nil)

	out, err := agentSvc.callContextRead(context.Background(), `{"resource":"schedules","action":"list"}`)
	if err != nil {
		t.Fatalf("callContextRead error: %v", err)
	}
	if !strings.Contains(out, `"status_code":200`) || !strings.Contains(out, `"path":"/api/schedules"`) {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCallContextRead_RejectsInvalidResource(t *testing.T) {
	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          2,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, conversation.NewStore(), &mockLLM{}, nil)

	_, err := agentSvc.callContextRead(context.Background(), `{"resource":"unknown","action":"list"}`)
	if err == nil || !strings.Contains(err.Error(), "unsupported resource") {
		t.Fatalf("expected unsupported resource error, got %v", err)
	}
}

func TestCallMaintenanceWrite_ValidatesRequiredPayload(t *testing.T) {
	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          2,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, conversation.NewStore(), &mockLLM{}, nil)

	_, err := agentSvc.callMaintenanceWrite(context.Background(), `{"resource":"schedules","operation":"save","payload":{"id":"daily"}}`)
	if err == nil || !strings.Contains(err.Error(), "payload field") {
		t.Fatalf("expected required field validation error, got %v", err)
	}
}

func TestCallMaintenanceWrite_SavesSchedule(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/schedules/save" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body error: %v", err)
		}
		if strings.TrimSpace(payload["id"].(string)) != "daily-task" {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(apiServer.Close)

	agentSvc := New(Config{
		Model:                      "test-model",
		LocalAPIBaseURL:            apiServer.URL,
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          2,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, conversation.NewStore(), &mockLLM{}, nil)

	out, err := agentSvc.callMaintenanceWrite(context.Background(), `{"resource":"schedules","operation":"save","payload":{"id":"daily-task","name":"daily","description":"desc","action":"skill:daily","cron_expr":"30 8 * * *","enabled":true}}`)
	if err != nil {
		t.Fatalf("callMaintenanceWrite error: %v", err)
	}
	if !strings.Contains(out, `"path":"/api/schedules/save"`) {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunLinuxBash_BlocksLocalAPICommand(t *testing.T) {
	_, err := runLinuxBash(context.Background(), linuxBashRequest{
		Command: "curl -sS http://127.0.0.1:8080/api/schedules",
	})
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected local api forbidden error, got %v", err)
	}
}
