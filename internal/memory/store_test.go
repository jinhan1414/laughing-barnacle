package memory

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreUpsertReadAndIndex(t *testing.T) {
	store, err := NewStoreWithFile(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	_, err = store.UpsertNode(UpsertRequest{
		Mode:          "replace",
		Path:          "/projects/pay-refactor/overview",
		Type:          NodeTypeFile,
		Title:         "项目概览",
		SchemaKind:    "project_overview",
		SchemaVersion: 1,
		Summary:       "支付重构灰度中",
		Facts:         []string{"灰度30%", "成功率99.98%"},
	})
	if err != nil {
		t.Fatalf("UpsertNode error: %v", err)
	}

	node, err := store.ReadNode("/projects/pay-refactor/overview")
	if err != nil {
		t.Fatalf("ReadNode error: %v", err)
	}
	if node.Type != NodeTypeFile || node.Content == nil {
		t.Fatalf("unexpected node: %+v", node)
	}
	if node.Content.Summary != "支付重构灰度中" {
		t.Fatalf("unexpected summary: %q", node.Content.Summary)
	}

	items, err := store.ListIndex("/projects/pay-refactor")
	if err != nil {
		t.Fatalf("ListIndex error: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected non-empty index")
	}
}

func TestSegmentIdleCloseAndPersist(t *testing.T) {
	store, err := NewStoreWithFile(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	if err := store.AppendTurn("你好", "你好，我在。", nil, base); err != nil {
		t.Fatalf("AppendTurn error: %v", err)
	}

	closed, err := store.CloseIdleSegments(base.Add(6*time.Minute), 5*time.Minute, 10*time.Minute, 8)
	if err != nil {
		t.Fatalf("CloseIdleSegments error: %v", err)
	}
	if len(closed) != 1 {
		t.Fatalf("expected one closed segment, got %d", len(closed))
	}

	if err := store.ProcessClosedSegments(base.Add(6 * time.Minute)); err != nil {
		t.Fatalf("ProcessClosedSegments error: %v", err)
	}

	node, err := store.ReadNode("/conversation/archive/" + closed[0].ID + "/index")
	if err != nil {
		t.Fatalf("Read archive index node error: %v", err)
	}
	if node.Content == nil || len(node.Content.Sections) == 0 {
		t.Fatalf("expected archive sections, got %+v", node)
	}
}
