package agent

import (
	"context"
	"testing"
	"time"
)

type metadataCaptureA2AProvider struct {
	sendResult A2ATaskResult
	lastSend   A2ASendRequest
}

func (p *metadataCaptureA2AProvider) ListIndexLines(_ int) []string { return nil }

func (p *metadataCaptureA2AProvider) Send(_ context.Context, req A2ASendRequest) (A2ATaskResult, error) {
	p.lastSend = req
	p.lastSend.Metadata = cloneAnyMap(req.Metadata)
	return p.sendResult, nil
}

func (p *metadataCaptureA2AProvider) GetTask(_ context.Context, _ A2ATaskQuery) (A2ATaskResult, error) {
	return A2ATaskResult{Status: "working"}, nil
}

func (p *metadataCaptureA2AProvider) CancelTask(_ context.Context, _ A2ATaskQuery) (A2ATaskResult, error) {
	return A2ATaskResult{Status: "canceled"}, nil
}

func TestA2ASendMetadataIncludesAsyncTaskID(t *testing.T) {
	manager := newAsyncTaskManager(nil, "", time.Now)
	provider := &metadataCaptureA2AProvider{
		sendResult: A2ATaskResult{AgentID: "codex-local", TaskID: "remote-meta", Status: "completed"},
	}
	manager.SetA2AProvider(provider)
	inputMetadata := map[string]any{
		"working_dir": "E:\\projects\\ai\\work-notiy",
		"trace_id":    "trace-123",
	}
	task, err := manager.Submit(context.Background(), AsyncTaskSubmitInput{
		TaskType:   asyncTaskTypeA2A,
		Request:    "metadata",
		AgentID:    "codex-local",
		AgentInput: "run",
		Metadata:   inputMetadata,
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	done := waitAsyncTaskTerminal(t, manager, task.ID)
	if done.Status != asyncTaskStatusSucceeded {
		t.Fatalf("expected succeeded status, got %+v", done)
	}
	sendMetadata := provider.lastSend.Metadata
	if sendMetadata["working_dir"] != inputMetadata["working_dir"] {
		t.Fatalf("working_dir metadata mismatch: %#v", sendMetadata)
	}
	if sendMetadata["trace_id"] != inputMetadata["trace_id"] {
		t.Fatalf("trace_id metadata mismatch: %#v", sendMetadata)
	}
	if sendMetadata["async_task_id"] != task.ID {
		t.Fatalf("async_task_id metadata mismatch: %#v", sendMetadata)
	}
	if _, exists := inputMetadata["async_task_id"]; exists {
		t.Fatalf("input metadata should not be mutated: %#v", inputMetadata)
	}
}
