package agent

import (
	"context"
	"strings"
	"testing"

	"laughing-barnacle/internal/conversation"
)

func TestOnAsyncTaskCompleted_AppendsSucceededResultSummary(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"async_task_notify": {"后台任务已完成。"},
		},
	}
	agentSvc := New(Config{Model: "test-model"}, store, fakeLLM, nil)

	agentSvc.onAsyncTaskCompleted(context.Background(), AsyncTask{
		ID:             "async_1",
		TaskType:       asyncTaskTypeA2A,
		Status:         asyncTaskStatusSucceeded,
		NotifyOnFinish: true,
		Result: "agent_id: codex-local\n" +
			"task_id: remote-1\n" +
			"status: completed\n" +
			"artifact: 技术栈：Go + bbolt\n" +
			"artifact: 核心模块：agent/web/memory\n",
	})

	_, messages := store.Snapshot()
	if len(messages) != 1 {
		t.Fatalf("expected one assistant message, got %d", len(messages))
	}
	content := strings.TrimSpace(messages[0].Content)
	if !strings.Contains(content, "后台任务已完成。") {
		t.Fatalf("expected base notification, got %q", content)
	}
	if !strings.Contains(content, "结果摘要：") {
		t.Fatalf("expected result summary in notification, got %q", content)
	}
	if !strings.Contains(content, "技术栈：Go + bbolt") {
		t.Fatalf("expected artifact details in summary, got %q", content)
	}
}

func TestOnAsyncTaskCompleted_WritesFallbackWithoutLLM(t *testing.T) {
	store := conversation.NewStore()
	agentSvc := New(Config{Model: "test-model"}, store, nil, nil)

	agentSvc.onAsyncTaskCompleted(context.Background(), AsyncTask{
		ID:             "async_2",
		Status:         asyncTaskStatusFailed,
		NotifyOnFinish: true,
		Error:          "network timeout",
	})

	_, messages := store.Snapshot()
	if len(messages) != 1 {
		t.Fatalf("expected one assistant message, got %d", len(messages))
	}
	content := strings.TrimSpace(messages[0].Content)
	if !strings.Contains(content, "后台任务 async_2 执行失败") {
		t.Fatalf("expected fallback failure notification, got %q", content)
	}
	if !strings.Contains(content, "失败原因：network timeout") {
		t.Fatalf("expected explicit failure reason summary, got %q", content)
	}
}

func TestOnAsyncTaskCompleted_SkipsWhenContextCanceled(t *testing.T) {
	store := conversation.NewStore()
	agentSvc := New(Config{Model: "test-model"}, store, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	agentSvc.onAsyncTaskCompleted(ctx, AsyncTask{
		ID:             "async_3",
		Status:         asyncTaskStatusFailed,
		NotifyOnFinish: true,
		Error:          "canceled",
	})

	_, messages := store.Snapshot()
	if len(messages) != 0 {
		t.Fatalf("expected no assistant message, got %d", len(messages))
	}
}

func TestOnAsyncTaskCompleted_SkipsNotificationWhenNotifyDisabled(t *testing.T) {
	store := conversation.NewStore()
	agentSvc := New(Config{Model: "test-model"}, store, nil, nil)

	agentSvc.onAsyncTaskCompleted(context.Background(), AsyncTask{
		ID:             "async_4",
		Status:         asyncTaskStatusSucceeded,
		NotifyOnFinish: false,
	})

	_, messages := store.Snapshot()
	if len(messages) != 0 {
		t.Fatalf("expected no assistant message when notify_on_finish=false, got %d", len(messages))
	}
}

func TestSummarizeSucceededTaskResult_PrefersMultilineArtifactAndSkipsEvidence(t *testing.T) {
	summary := summarizeSucceededTaskResult("agent_id: codex-local\n" +
		"task_id: remote-1\n" +
		"status: completed\n" +
		"artifact: **结论**\n" +
		"- 项目可正常构建与运行\n" +
		"- 主要风险是配置硬编码\n" +
		"artifact: {\"working_dir\":\"E:/projects/ai/work-notiy\",\"events_file\":\"E:/tmp/out.events.jsonl\",\"event_count\":12,\"turn_completed\":true}\n")

	if !strings.Contains(summary, "**结论**") {
		t.Fatalf("expected multiline artifact title in summary, got %q", summary)
	}
	if !strings.Contains(summary, "项目可正常构建与运行") {
		t.Fatalf("expected multiline artifact body in summary, got %q", summary)
	}
	if strings.Contains(summary, "working_dir") {
		t.Fatalf("expected evidence artifact skipped, got %q", summary)
	}
}
