package web

import (
	"testing"
	"time"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/memory"
)

func TestBuildChatTimeline_MergeAndSort(t *testing.T) {
	base := time.Now()
	msgs := []conversation.Message{
		{Role: "user", Content: "u1", CreatedAt: base.Add(1 * time.Second)},
		{Role: "assistant", Content: "a1", CreatedAt: base.Add(3 * time.Second)},
		{Role: "system", Content: "ignore", CreatedAt: base.Add(2 * time.Second)},
	}
	events := []conversation.Event{
		{Type: "context_compression", Content: "c1", CreatedAt: base.Add(2 * time.Second)},
		{Type: "other", Content: "ignore", CreatedAt: base.Add(4 * time.Second)},
	}

	got := buildChatTimeline(msgs, events)
	if len(got) != 3 {
		t.Fatalf("expected 3 timeline items, got %d", len(got))
	}
	if got[0].Kind != "user" || got[0].Content != "u1" {
		t.Fatalf("unexpected first item: %+v", got[0])
	}
	if got[1].Kind != "event" || got[1].Content != "c1" {
		t.Fatalf("unexpected second item: %+v", got[1])
	}
	if got[2].Kind != "assistant" || got[2].Content != "a1" {
		t.Fatalf("unexpected third item: %+v", got[2])
	}
	if !got[0].ShowTimestamp {
		t.Fatalf("expected first item to show timestamp")
	}
	if got[1].ShowTimestamp {
		t.Fatalf("expected second item to hide timestamp")
	}
	if got[2].ShowTimestamp {
		t.Fatalf("expected third item to hide timestamp")
	}
}

func TestShouldShowChatTimestamp(t *testing.T) {
	loc := time.Local
	base := time.Date(2026, 2, 18, 10, 0, 0, 0, loc)

	if !shouldShowChatTimestamp(time.Time{}, base) {
		t.Fatalf("expected first timestamp to be shown")
	}
	if shouldShowChatTimestamp(base, base.Add(4*time.Minute)) {
		t.Fatalf("expected timestamp to stay hidden within 5 minutes")
	}
	if !shouldShowChatTimestamp(base, base.Add(5*time.Minute)) {
		t.Fatalf("expected timestamp to be shown at 5-minute gap")
	}
	if !shouldShowChatTimestamp(base, base.Add(14*time.Hour)) {
		t.Fatalf("expected timestamp to be shown across different days")
	}
}

func TestFormatChatTimestamp(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 2, 18, 10, 30, 0, 0, loc)

	today := time.Date(2026, 2, 18, 9, 5, 0, 0, loc)
	if got := formatChatTimestamp(today, now); got != "09:05" {
		t.Fatalf("unexpected today label: %s", got)
	}

	yesterday := time.Date(2026, 2, 17, 21, 8, 0, 0, loc)
	if got := formatChatTimestamp(yesterday, now); got != "昨天 21:08" {
		t.Fatalf("unexpected yesterday label: %s", got)
	}

	sameYear := time.Date(2026, 1, 2, 8, 7, 0, 0, loc)
	if got := formatChatTimestamp(sameYear, now); got != "1月2日 08:07" {
		t.Fatalf("unexpected same-year label: %s", got)
	}

	lastYear := time.Date(2025, 12, 31, 23, 59, 0, 0, loc)
	if got := formatChatTimestamp(lastYear, now); got != "2025年12月31日 23:59" {
		t.Fatalf("unexpected cross-year label: %s", got)
	}
}

func TestLatestMaintenanceAudit(t *testing.T) {
	now := time.Now().UTC()
	entries := []memory.AuditEntry{
		{ID: "a1", Action: "upsert", CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "a2", Action: "maintenance", Detail: "retried=1", CreatedAt: now.Add(-3 * time.Minute)},
		{ID: "a3", Action: "maintenance", Detail: "retried=2", CreatedAt: now.Add(-1 * time.Minute)},
	}

	entry, ok := latestMaintenanceAudit(entries)
	if !ok {
		t.Fatalf("expected to find maintenance entry")
	}
	if entry.ID != "a3" {
		t.Fatalf("expected latest maintenance entry a3, got %s", entry.ID)
	}
	if entry.Detail != "retried=2" {
		t.Fatalf("unexpected maintenance detail: %s", entry.Detail)
	}

	if _, ok := latestMaintenanceAudit([]memory.AuditEntry{{ID: "b1", Action: "upsert", CreatedAt: now}}); ok {
		t.Fatalf("expected no maintenance entry")
	}
}
