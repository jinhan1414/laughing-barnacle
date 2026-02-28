package agent

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestA2AStatusMappingMatrix(t *testing.T) {
	tests := []struct {
		status        string
		inProgress    bool
		terminal      string
		blockedReason string
	}{
		{status: "submitted", inProgress: true},
		{status: "working", inProgress: true},
		{status: "input-required", blockedReason: "a2a task requires additional input"},
		{status: "auth-required", blockedReason: "a2a task requires authentication"},
		{status: "completed", terminal: asyncTaskStatusSucceeded},
		{status: "failed", terminal: asyncTaskStatusFailed},
		{status: "canceled", terminal: asyncTaskStatusCanceled},
		{status: "rejected", terminal: asyncTaskStatusFailed},
	}

	for _, tt := range tests {
		if got := isA2AInProgressStatus(tt.status); got != tt.inProgress {
			t.Fatalf("status=%q inProgress=%v want=%v", tt.status, got, tt.inProgress)
		}
		if got := normalizeA2ATerminalStatus(tt.status); got != tt.terminal {
			t.Fatalf("status=%q terminal=%q want=%q", tt.status, got, tt.terminal)
		}
		if got := normalizeA2ABlockedStatus(tt.status); got != tt.blockedReason {
			t.Fatalf("status=%q blocked=%q want=%q", tt.status, got, tt.blockedReason)
		}
	}
}

func TestRenderA2ATaskResult_IncludesProtocolEvidence(t *testing.T) {
	rendered := renderA2ATaskResult(A2ATaskResult{
		AgentID:     "codex-local",
		TaskID:      "task-1",
		Status:      "failed",
		RawStatus:   "rejected",
		SDKProvider: "a2a-go",
		SDKMethod:   "SendMessage",
		Message:     "rejected by policy",
	})
	for _, expected := range []string{
		"agent_id: codex-local",
		"task_id: task-1",
		"status: failed",
		"raw_status: rejected",
		"sdk_provider: a2a-go",
		"sdk_method: SendMessage",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("render missing evidence %q in %q", expected, rendered)
		}
	}
}

func TestRunA2ATask_InputRequiredFailsExplicitly(t *testing.T) {
	manager := newAsyncTaskManager(nil, "", time.Now)
	manager.SetA2AProvider(&mockA2A{
		send: A2ATaskResult{
			AgentID:     "codex-local",
			TaskID:      "remote-1",
			Status:      "input-required",
			RawStatus:   "input-required",
			SDKProvider: "a2a-go",
			SDKMethod:   "SendMessage",
			Message:     "need more info",
		},
	})

	task, err := manager.Submit(context.Background(), AsyncTaskSubmitInput{
		TaskType:   asyncTaskTypeA2A,
		Request:    "run a2a",
		AgentID:    "codex-local",
		AgentInput: "hello",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	finished := waitAsyncTaskTerminal(t, manager, task.ID)
	if finished.Status != asyncTaskStatusFailed {
		t.Fatalf("expected failed status, got %#v", finished)
	}
	if !strings.Contains(finished.Error, "requires additional input") {
		t.Fatalf("expected explicit blocked error, got %#v", finished)
	}
	if !strings.Contains(finished.Result, "raw_status: input-required") {
		t.Fatalf("expected raw_status evidence, got %#v", finished)
	}
}

func waitAsyncTaskTerminal(t *testing.T, manager *AsyncTaskManager, taskID string) AsyncTask {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := manager.Get(AsyncTaskGetInput{TaskID: taskID})
		if err == nil && isAsyncTaskTerminal(task.Status) {
			return task
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("task %s did not finish before timeout", taskID)
	return AsyncTask{}
}
