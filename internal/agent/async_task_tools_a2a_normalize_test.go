package agent

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCallAsyncTaskSubmit_NormalizesA2ASelfDispatchPhrases(t *testing.T) {
	manager := newAsyncTaskManager(nil, "", time.Now)
	provider := &metadataCaptureA2AProvider{
		sendResult: A2ATaskResult{AgentID: "codex-local", TaskID: "remote-self", Status: "completed"},
	}
	manager.SetA2AProvider(provider)
	agentSvc := &Agent{asyncTasks: manager}

	raw := `{"task_type":"a2a","request":"再次重新调用 codex-local 分析项目 E:\\projects\\ai\\work-notiy","agent_id":"codex-local","agent_input":"请调用 codex-local 分析本地项目 E:\\projects\\ai\\work-notiy","metadata":{"working_dir":"E:\\projects\\ai\\work-notiy"}}`
	output, err := agentSvc.callAsyncTaskSubmit(context.Background(), raw)
	if err != nil {
		t.Fatalf("callAsyncTaskSubmit failed: %v", err)
	}

	taskID := extractAsyncTaskID(output)
	if taskID == "" {
		t.Fatalf("missing async_task_id in output: %q", output)
	}
	done := waitAsyncTaskTerminal(t, manager, taskID)
	if done.Status != asyncTaskStatusSucceeded {
		t.Fatalf("expected succeeded task, got %#v", done)
	}
	if strings.Contains(done.Request, "codex-local") || strings.Contains(done.Request, "调用 codex-local") {
		t.Fatalf("request not normalized: %q", done.Request)
	}
	if strings.Contains(provider.lastSend.Message, "codex-local") || strings.Contains(provider.lastSend.Message, "调用 codex-local") {
		t.Fatalf("agent_input not normalized before send: %q", provider.lastSend.Message)
	}
}

func extractAsyncTaskID(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "async_task_id:") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "async_task_id:"))
	}
	return ""
}
