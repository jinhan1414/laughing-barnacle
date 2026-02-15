package llmlog

import (
	"path/filepath"
	"testing"
)

func TestStoreWithFilePersistsEntries(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "llm_logs.json")

	store, err := NewStoreWithFile(5, logPath)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}

	store.Add(Entry{Purpose: "chat_reply", Request: "req-1"})
	store.Add(Entry{Purpose: "compress_context", Request: "req-2"})

	reloaded, err := NewStoreWithFile(5, logPath)
	if err != nil {
		t.Fatalf("reload store failed: %v", err)
	}

	entries := reloaded.List()
	if got := len(entries); got != 2 {
		t.Fatalf("expected 2 entries, got %d", got)
	}
	if entries[0].Purpose != "compress_context" || entries[1].Purpose != "chat_reply" {
		t.Fatalf("unexpected entry order: %+v", entries)
	}

	reloaded.Add(Entry{Purpose: "retry", Request: "req-3"})
	afterAppend := reloaded.List()
	if afterAppend[0].ID != 3 {
		t.Fatalf("expected id to continue from persisted entries, got %d", afterAppend[0].ID)
	}
}

func TestStoreWithFileRespectsLimit(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "llm_logs.json")

	store, err := NewStoreWithFile(2, logPath)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}

	store.Add(Entry{Purpose: "p1"})
	store.Add(Entry{Purpose: "p2"})
	store.Add(Entry{Purpose: "p3"})

	reloaded, err := NewStoreWithFile(2, logPath)
	if err != nil {
		t.Fatalf("reload store failed: %v", err)
	}

	entries := reloaded.List()
	if got := len(entries); got != 2 {
		t.Fatalf("expected 2 entries, got %d", got)
	}
	if entries[0].Purpose != "p3" || entries[1].Purpose != "p2" {
		t.Fatalf("unexpected entries after limit trim: %+v", entries)
	}
}

func TestStoreClear_ClearsPersistedEntriesAndResetsID(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "llm_logs.json")

	store, err := NewStoreWithFile(5, logPath)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}

	store.Add(Entry{Purpose: "p1"})
	store.Add(Entry{Purpose: "p2"})

	if err := store.Clear(); err != nil {
		t.Fatalf("Clear error: %v", err)
	}
	if got := len(store.List()); got != 0 {
		t.Fatalf("expected no entries after clear, got %d", got)
	}

	store.Add(Entry{Purpose: "p3"})
	entries := store.List()
	if got := len(entries); got != 1 {
		t.Fatalf("expected one entry after re-add, got %d", got)
	}
	if entries[0].ID != 1 {
		t.Fatalf("expected id reset to 1, got %d", entries[0].ID)
	}

	reloaded, err := NewStoreWithFile(5, logPath)
	if err != nil {
		t.Fatalf("reload store failed: %v", err)
	}
	entries = reloaded.List()
	if got := len(entries); got != 1 {
		t.Fatalf("expected one persisted entry after clear+add, got %d", got)
	}
	if entries[0].Purpose != "p3" || entries[0].ID != 1 {
		t.Fatalf("unexpected persisted entry after clear: %+v", entries[0])
	}
}
