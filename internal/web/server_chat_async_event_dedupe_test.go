package web

import (
	"strings"
	"testing"
	"time"

	"laughing-barnacle/internal/conversation"
)

func TestBuildChatTimeline_DedupesAsyncTaskStatusByTaskID(t *testing.T) {
	msgTime := time.Now()
	events := []conversation.Event{
		{
			Type:      "async_task_status",
			Content:   "task_id=async_1 | type=a2a | status=submitted",
			CreatedAt: msgTime,
		},
		{
			Type:      "async_task_status",
			Content:   "task_id=async_1 | type=a2a | status=working",
			CreatedAt: msgTime.Add(2 * time.Second),
		},
	}

	timeline := buildChatTimeline(nil, events)
	if len(timeline) != 1 {
		t.Fatalf("expected one timeline item after dedupe, got %d", len(timeline))
	}
	item := timeline[0]
	if item.Kind != "event" || item.EventType != "async_task_status" {
		t.Fatalf("unexpected timeline item: %+v", item)
	}
	if item.EventTaskID != "async_1" {
		t.Fatalf("expected event task id async_1, got %q", item.EventTaskID)
	}
	if !strings.Contains(item.Content, "status=working") {
		t.Fatalf("expected latest event content, got %q", item.Content)
	}
}
