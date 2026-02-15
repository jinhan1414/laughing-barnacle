package conversation

import (
	"path/filepath"
	"testing"
)

func TestStoreWithFile_PersistsSummaryMessagesAndToolCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.json")
	store, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}

	store.Append("user", "今天北京天气")
	if err := store.SetLatestUserToolCalls([]ToolCall{
		{
			ID:        "call_1",
			Name:      "weather__query",
			Arguments: `{"city":"beijing"}`,
			Result:    `{"temp":18}`,
		},
	}); err != nil {
		t.Fatalf("SetLatestUserToolCalls error: %v", err)
	}
	store.Append("assistant", "18 度")
	store.SetSummaryAndTrim("用户询问天气", 10)
	store.AppendEvent("context_compression", "【上下文压缩】\n用户询问天气")

	reloaded, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}

	summary, messages, events := reloaded.SnapshotWithEvents()
	if summary != "用户询问天气" {
		t.Fatalf("unexpected summary: %q", summary)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 1 {
		t.Fatalf("expected one tool call on first user message, got %d", len(messages[0].ToolCalls))
	}
	if messages[0].ToolCalls[0].Name != "weather__query" {
		t.Fatalf("unexpected tool call name: %q", messages[0].ToolCalls[0].Name)
	}
	if messages[1].Role != "assistant" {
		t.Fatalf("unexpected second role: %s", messages[1].Role)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "context_compression" {
		t.Fatalf("unexpected event type: %q", events[0].Type)
	}
}

func TestSetLatestUserToolCalls_RequiresPendingUserMessage(t *testing.T) {
	store := NewStore()
	store.Append("assistant", "ready")

	if err := store.SetLatestUserToolCalls([]ToolCall{{Name: "any"}}); err == nil {
		t.Fatalf("expected error without pending user message")
	}
}

func TestStoreReset_ClearsAndPersistsAllState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.json")
	store, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}

	store.Append("user", "u1")
	store.Append("assistant", "a1")
	store.SetSummaryAndTrim("summary", 10)
	store.AppendEvent("context_compression", "compressed")

	if err := store.Reset(); err != nil {
		t.Fatalf("Reset error: %v", err)
	}

	summary, messages, events := store.SnapshotWithEvents()
	if summary != "" {
		t.Fatalf("expected empty summary after reset, got %q", summary)
	}
	if len(messages) != 0 {
		t.Fatalf("expected no messages after reset, got %d", len(messages))
	}
	if len(events) != 0 {
		t.Fatalf("expected no events after reset, got %d", len(events))
	}

	reloaded, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}
	summary, messages, events = reloaded.SnapshotWithEvents()
	if summary != "" || len(messages) != 0 || len(events) != 0 {
		t.Fatalf("expected persisted empty state, got summary=%q messages=%d events=%d", summary, len(messages), len(events))
	}
}
