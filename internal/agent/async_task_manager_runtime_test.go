package agent

import (
	"context"
	"testing"
	"time"
)

func TestAsyncTaskCompletionHookRunsWhenNotifyDisabled(t *testing.T) {
	manager := newAsyncTaskManager(nil, "", time.Now)
	doneCh := make(chan AsyncTask, 1)
	manager.SetHooks(nil, func(_ context.Context, task AsyncTask) {
		doneCh <- task
	})

	task, err := manager.Submit(context.Background(), AsyncTaskSubmitInput{
		TaskType:       asyncTaskTypeGeneric,
		Request:        "run generic",
		NotifyOnFinish: false,
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	select {
	case done := <-doneCh:
		if done.ID != task.ID {
			t.Fatalf("unexpected callback task id: %s", done.ID)
		}
		if !isAsyncTaskTerminal(done.Status) {
			t.Fatalf("expected terminal task status, got %+v", done)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected completion hook callback")
	}
}
