package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCallAsyncTaskSubmit_DerivesWorkingDirFromProjectID(t *testing.T) {
	manager := newAsyncTaskManager(nil, "", time.Now)
	provider := &metadataCaptureA2AProvider{
		sendResult: A2ATaskResult{AgentID: codexLocalAgentID, TaskID: "remote-project", Status: "completed"},
	}
	manager.SetA2AProvider(provider)
	agentSvc := &Agent{asyncTasks: manager, memory: &mockMemory{
		projectDirs: map[string]string{
			"laughing-barnacle": `E:\projects\ai\laughing-barnacle`,
		},
	}, projectCfg: &mockProjectRoot{rootDir: `E:\projects\ai`}}

	_, err := agentSvc.callAsyncTaskSubmit(context.Background(), `{"task_type":"a2a","request":"修复 bug","agent_id":"codex-local","agent_input":"请修复 bug","metadata":{"project_id":"laughing-barnacle"}}`)
	if err != nil {
		t.Fatalf("callAsyncTaskSubmit failed: %v", err)
	}
	waitAsyncTaskTerminal(t, manager, manager.ListTasks()[0].ID)
	if got := provider.lastSend.Metadata["working_dir"]; got != `E:\projects\ai\laughing-barnacle` {
		t.Fatalf("expected derived working_dir, got %#v", provider.lastSend.Metadata)
	}
}

func TestCallAsyncTaskSubmit_CreatesNewProjectAndUpsertsRegistry(t *testing.T) {
	rootDir := t.TempDir()
	manager := newAsyncTaskManager(nil, "", time.Now)
	provider := &metadataCaptureA2AProvider{
		sendResult: A2ATaskResult{AgentID: codexLocalAgentID, TaskID: "remote-new", Status: "completed"},
	}
	memory := &mockMemory{}
	manager.SetA2AProvider(provider)
	agentSvc := &Agent{asyncTasks: manager, memory: memory, projectCfg: &mockProjectRoot{rootDir: rootDir}}

	_, err := agentSvc.callAsyncTaskSubmit(context.Background(), `{"task_type":"a2a","request":"新建项目","agent_id":"codex-local","agent_input":"请初始化项目","metadata":{"project_id":"demo-app","create_project":true,"project_summary":"demo"}}`)
	if err != nil {
		t.Fatalf("callAsyncTaskSubmit failed: %v", err)
	}
	waitAsyncTaskTerminal(t, manager, manager.ListTasks()[0].ID)
	expected := filepath.Join(rootDir, "demo-app")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected created project dir, got %v", err)
	}
	if got := provider.lastSend.Metadata["working_dir"]; got != expected {
		t.Fatalf("expected created working_dir %q, got %#v", expected, provider.lastSend.Metadata)
	}
	if len(memory.upsertedProjects) != 1 {
		t.Fatalf("expected project registry upsert, got %#v", memory.upsertedProjects)
	}
}

func TestCallAsyncTaskSubmit_CodexLocalRejectsMissingProjectContext(t *testing.T) {
	agentSvc := &Agent{asyncTasks: newAsyncTaskManager(nil, "", time.Now)}
	if _, err := agentSvc.callAsyncTaskSubmit(context.Background(), `{"task_type":"a2a","request":"修复 bug","agent_id":"codex-local","agent_input":"请修复 bug"}`); err == nil {
		t.Fatalf("expected missing project context error")
	}
}
