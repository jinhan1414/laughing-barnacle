package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type stubExtractor struct {
	extraction SegmentExtraction
	err        error
}

func (s stubExtractor) ExtractSegment(_ context.Context, _ Segment) (SegmentExtraction, error) {
	if s.err != nil {
		return SegmentExtraction{}, s.err
	}
	return s.extraction, nil
}

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

	projectNode, err := store.ReadNode("/projects/session-journal/" + closed[0].ID)
	if err != nil {
		t.Fatalf("Read project journal node error: %v", err)
	}
	if projectNode.Content == nil || strings.TrimSpace(projectNode.Content.Summary) == "" {
		t.Fatalf("expected project journal summary, got %+v", projectNode)
	}

	pending := store.ListInboxPending(20)
	if len(pending) == 0 {
		t.Fatalf("expected pending inbox candidates")
	}

	confirmedPath, err := store.ReviewInboxCandidate(pending[0].Path, "confirm")
	if err != nil {
		t.Fatalf("ReviewInboxCandidate confirm error: %v", err)
	}
	confirmedNode, err := store.ReadNode(confirmedPath)
	if err != nil {
		t.Fatalf("Read confirmed node error: %v", err)
	}
	if confirmedNode.Content == nil {
		t.Fatalf("expected confirmed file content")
	}

	if len(pending) > 1 {
		rejectedPath, err := store.ReviewInboxCandidate(pending[1].Path, "reject")
		if err != nil {
			t.Fatalf("ReviewInboxCandidate reject error: %v", err)
		}
		if !strings.HasPrefix(rejectedPath, "/inbox/trash/") {
			t.Fatalf("expected reject moved to trash, got %s", rejectedPath)
		}
	}
}

func TestRunMaintenanceRetryAndCleanup(t *testing.T) {
	store, err := NewStoreWithFile(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	seg := Segment{
		ID:             "seg-maintenance-1",
		Status:         SegmentStatusFailed,
		Turns:          []SegmentTurn{{User: "更新项目状态", Assistant: "收到", CreatedAt: base}},
		StartedAt:      base,
		LastUserAt:     base,
		LastActivityAt: base,
		UpdatedAt:      base,
		CreatedAt:      base,
	}
	if err := store.writeSegmentLocked(seg); err != nil {
		t.Fatalf("writeSegmentLocked error: %v", err)
	}

	trashNode, err := store.UpsertNode(UpsertRequest{
		Mode:          "replace",
		Path:          "/inbox/trash/old-candidate",
		Type:          NodeTypeFile,
		Title:         "old",
		SchemaKind:    "trash_item",
		SchemaVersion: 1,
		Summary:       "old item",
		Source:        "system",
		Confidence:    1,
	})
	if err != nil {
		t.Fatalf("Upsert trash node error: %v", err)
	}
	trashNode.UpdatedAt = base.Add(-40 * 24 * time.Hour)
	if err := store.writeNodeLocked(trashNode); err != nil {
		t.Fatalf("writeNodeLocked error: %v", err)
	}

	report, err := store.RunMaintenance(base.Add(10*time.Minute), 30*24*time.Hour, time.Minute)
	if err != nil {
		t.Fatalf("RunMaintenance error: %v", err)
	}
	if report.RetriedSegments == 0 {
		t.Fatalf("expected retried segments > 0")
	}
	if report.RemovedTrashNodes == 0 {
		t.Fatalf("expected removed trash nodes > 0")
	}

	if _, err := store.ReadNode("/inbox/trash/old-candidate"); err == nil {
		t.Fatalf("expected old trash node removed")
	}

	metrics := store.GetMetrics()
	if metrics.SegmentTotal == 0 {
		t.Fatalf("expected segment metrics")
	}
	if metrics.SegmentPersisted == 0 {
		t.Fatalf("expected persisted metrics")
	}
	if metrics.ReviewedCount == 0 && metrics.PendingCount == 0 {
		t.Fatalf("expected inbox metrics")
	}
}

func TestProcessClosedSegments_ExtractorFallback(t *testing.T) {
	store, err := NewStoreWithFile(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	store.SetSegmentExtractor(stubExtractor{err: context.DeadlineExceeded}, true)

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	if err := store.AppendTurn("需要更新目标", "收到", nil, base); err != nil {
		t.Fatalf("AppendTurn error: %v", err)
	}
	if _, err := store.CloseIdleSegments(base.Add(6*time.Minute), 5*time.Minute, 10*time.Minute, 8); err != nil {
		t.Fatalf("CloseIdleSegments error: %v", err)
	}
	if err := store.ProcessClosedSegments(base.Add(6 * time.Minute)); err != nil {
		t.Fatalf("ProcessClosedSegments error: %v", err)
	}

	segments := store.ListSegments(10)
	if len(segments) == 0 || segments[0].Status != SegmentStatusPersisted {
		t.Fatalf("expected persisted segment with fallback, got %+v", segments)
	}
}

func TestProcessClosedSegments_ExtractorNoFallback(t *testing.T) {
	store, err := NewStoreWithFile(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStoreWithFile error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	store.SetSegmentExtractor(stubExtractor{err: context.DeadlineExceeded}, false)

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	if err := store.AppendTurn("需要更新目标", "收到", nil, base); err != nil {
		t.Fatalf("AppendTurn error: %v", err)
	}
	if _, err := store.CloseIdleSegments(base.Add(6*time.Minute), 5*time.Minute, 10*time.Minute, 8); err != nil {
		t.Fatalf("CloseIdleSegments error: %v", err)
	}
	if err := store.ProcessClosedSegments(base.Add(6 * time.Minute)); err != nil {
		t.Fatalf("ProcessClosedSegments error: %v", err)
	}

	segments := store.ListSegments(10)
	if len(segments) == 0 || segments[0].Status != SegmentStatusFailed {
		t.Fatalf("expected failed segment without fallback, got %+v", segments)
	}
}
