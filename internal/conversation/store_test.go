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

	reloaded, err := NewStoreWithFile(path)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}

	summary, messages := reloaded.Snapshot()
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
}

func TestSetLatestUserToolCalls_RequiresPendingUserMessage(t *testing.T) {
	store := NewStore()
	store.Append("assistant", "ready")

	if err := store.SetLatestUserToolCalls([]ToolCall{{Name: "any"}}); err == nil {
		t.Fatalf("expected error without pending user message")
	}
}
