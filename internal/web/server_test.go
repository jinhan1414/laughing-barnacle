package web

import (
	"testing"
	"time"

	"laughing-barnacle/internal/conversation"
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
}
