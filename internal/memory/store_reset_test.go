package memory

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreReset_ClearsDataAndKeepsDefaultNamespaces(t *testing.T) {
	store, err := NewStoreWithFile(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.UpsertNode(UpsertRequest{
		Mode:          "replace",
		Path:          "/projects/reset-case/overview",
		Type:          NodeTypeFile,
		Title:         "重置前概览",
		SchemaKind:    "project_overview",
		SchemaVersion: 1,
		Summary:       "重置后应删除",
	}); err != nil {
		t.Fatalf("UpsertNode error: %v", err)
	}

	base := time.Date(2026, 2, 26, 9, 0, 0, 0, time.UTC)
	if err := store.AppendTurn("hello", "world", nil, base); err != nil {
		t.Fatalf("AppendTurn error: %v", err)
	}
	if got := len(store.ListSegments(10)); got == 0 {
		t.Fatalf("expected segments before reset")
	}

	if err := store.Reset(); err != nil {
		t.Fatalf("Reset error: %v", err)
	}

	if _, err := store.ReadNode("/projects/reset-case/overview"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound after reset, got %v", err)
	}
	if got := len(store.ListSegments(10)); got != 0 {
		t.Fatalf("expected empty segments after reset, got %d", got)
	}
	for _, path := range defaultNamespacePaths {
		node, err := store.ReadNode(path)
		if err != nil {
			t.Fatalf("expected default path %s exists, got error: %v", path, err)
		}
		if node.Type != NodeTypeDir {
			t.Fatalf("expected default path %s to be dir, got %s", path, node.Type)
		}
	}
}
