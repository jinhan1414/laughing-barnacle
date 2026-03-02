package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

func TestAutonomousRunCheckpoint_CreateAndUpdate(t *testing.T) {
	manager := newAutonomousRunManager(time.Now)

	created, err := manager.Checkpoint(AutonomousRunCheckpointInput{
		Goal:        "自动运营 7 天",
		Status:      autonomousRunStatusWaitingAsync,
		CurrentStep: "generate_topics",
		WaitingRef:  "async_1",
		StepSummary: "已提交选题生成任务",
		ContextPatch: map[string]any{
			"day": 1,
		},
	})
	if err != nil {
		t.Fatalf("create checkpoint error: %v", err)
	}
	if created.ID == "" || created.WaitingRef != "async_1" || created.Status != autonomousRunStatusWaitingAsync {
		t.Fatalf("unexpected created run: %+v", created)
	}

	updated, err := manager.Checkpoint(AutonomousRunCheckpointInput{
		RunID:       created.ID,
		Goal:        created.Goal,
		Status:      autonomousRunStatusCompleted,
		CurrentStep: "day_1_done",
		StepSummary: "已完成第 1 天内容发布",
		ContextPatch: map[string]any{
			"published_posts": 1,
		},
	})
	if err != nil {
		t.Fatalf("update checkpoint error: %v", err)
	}
	if updated.ID != created.ID || updated.Status != autonomousRunStatusCompleted {
		t.Fatalf("unexpected updated run: %+v", updated)
	}
	if got := updated.Context["published_posts"]; got != 1 {
		t.Fatalf("expected merged context patch, got %#v", updated.Context)
	}
}

func TestAutonomousRunCheckpoint_RejectsWaitingAsyncWithoutRef(t *testing.T) {
	manager := newAutonomousRunManager(time.Now)
	_, err := manager.Checkpoint(AutonomousRunCheckpointInput{
		Goal:        "自动运营 7 天",
		Status:      autonomousRunStatusWaitingAsync,
		CurrentStep: "generate_topics",
	})
	if err == nil || !strings.Contains(err.Error(), "waiting_ref") {
		t.Fatalf("expected waiting_ref validation error, got %v", err)
	}
}

func TestResumeAutonomousRun_UpdatesWaitingRun(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "已继续下一步"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "call_checkpoint_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      builtinAutonomousRunCheckpointToolName,
							Arguments: `{"run_id":null,"goal":"invalid","status":"completed","current_step":"ignore","waiting_ref":null,"step_summary":null,"error":null,"context_patch":null}`,
						},
					},
				},
				nil,
			},
		},
	}
	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          2,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)

	run, err := agentSvc.runs.Checkpoint(AutonomousRunCheckpointInput{
		Goal:        "自动运营 7 天",
		Status:      autonomousRunStatusWaitingAsync,
		CurrentStep: "generate_topics",
		WaitingRef:  "async_1",
		StepSummary: "等待选题生成完成",
	})
	if err != nil {
		t.Fatalf("seed run error: %v", err)
	}
	fakeLLM.toolCalls["chat_reply"][0][0].Function.Arguments =
		`{"run_id":"` + run.ID + `","goal":"自动运营 7 天","status":"completed","current_step":"day_1_done","waiting_ref":null,"step_summary":"已完成第 1 天自动运营","error":null,"context_patch":{"published_posts":1}}`

	agentSvc.resumeAutonomousRun(AsyncTask{
		ID:       "async_1",
		TaskType: asyncTaskTypeA2A,
		Status:   asyncTaskStatusSucceeded,
		Result:   "artifact: 已生成选题",
	}, run)

	updated, err := agentSvc.runs.Get(run.ID)
	if err != nil {
		t.Fatalf("get updated run error: %v", err)
	}
	if updated.Status != autonomousRunStatusCompleted {
		t.Fatalf("expected completed run, got %+v", updated)
	}
	_, messages := store.Snapshot()
	if len(messages) == 0 || !strings.Contains(messages[len(messages)-1].Content, "已继续下一步") {
		t.Fatalf("expected assistant follow-up message, got %+v", messages)
	}
}

func TestResumeAutonomousRun_MissingCheckpointFailsRun(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"未写 checkpoint"},
		},
	}
	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          1,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)

	run, err := agentSvc.runs.Checkpoint(AutonomousRunCheckpointInput{
		Goal:        "自动运营 7 天",
		Status:      autonomousRunStatusWaitingAsync,
		CurrentStep: "generate_topics",
		WaitingRef:  "async_2",
		StepSummary: "等待选题生成完成",
	})
	if err != nil {
		t.Fatalf("seed run error: %v", err)
	}

	agentSvc.resumeAutonomousRun(AsyncTask{
		ID:     "async_2",
		Status: asyncTaskStatusSucceeded,
	}, run)

	updated, err := agentSvc.runs.Get(run.ID)
	if err != nil {
		t.Fatalf("get updated run error: %v", err)
	}
	if updated.Status != autonomousRunStatusFailed || !strings.Contains(updated.Error, "missing checkpoint") {
		t.Fatalf("expected failed run due to missing checkpoint, got %+v", updated)
	}
}

func TestHandleUserMessage_InjectsAutonomousRunIndex(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"chat_reply": {"ok"},
	}}
	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          1,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)
	_, err := agentSvc.runs.Checkpoint(AutonomousRunCheckpointInput{
		Goal:        "自动运营 7 天",
		Status:      autonomousRunStatusWaitingAsync,
		CurrentStep: "generate_topics",
		WaitingRef:  "async_1",
		StepSummary: "等待异步选题",
	})
	if err != nil {
		t.Fatalf("seed run error: %v", err)
	}

	if _, err := agentSvc.HandleUserMessage(context.Background(), "继续看当前进度"); err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	found := false
	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "Autonomous Runs 索引") {
			found = strings.Contains(msg.Content, "autonomous_run__checkpoint") &&
				strings.Contains(msg.Content, `context__read(resource="runs", action="get", id="<run_id>")`)
			break
		}
	}
	if !found {
		t.Fatalf("expected autonomous run index prompt in request messages")
	}
}
