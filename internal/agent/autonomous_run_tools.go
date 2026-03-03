package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"laughing-barnacle/internal/llm"
)

const builtinAutonomousRunCheckpointToolName = "autonomous_run__checkpoint"

func autonomousRunBuiltinToolDefinitions() []llm.ToolDefinition {
	return []llm.ToolDefinition{autonomousRunCheckpointToolDefinition()}
}

func autonomousRunCheckpointToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionDefinition{
			Name:        builtinAutonomousRunCheckpointToolName,
			Description: "Create or update one autonomous run checkpoint. Use it when a task needs multi-step automatic continuation.",
			Parameters: strictToolParameters(
				map[string]any{
					"run_id": map[string]any{"type": "string", "description": "Existing run id. Null when creating a new run."},
					"goal":   map[string]any{"type": "string", "description": "Overall multi-step goal. Required when creating a new run."},
					"status": map[string]any{"type": "string", "enum": []string{
						autonomousRunStatusRunning,
						autonomousRunStatusWaitingAsync,
						autonomousRunStatusWaitingHuman,
						autonomousRunStatusCompleted,
						autonomousRunStatusFailed,
					}},
					"current_step":  map[string]any{"type": "string", "description": "Current active step label."},
					"waiting_ref":   map[string]any{"type": "string", "description": "Required when status=waiting_async."},
					"step_summary":  map[string]any{"type": "string", "description": "Short summary for the checkpoint trail."},
					"error":         map[string]any{"type": "string", "description": "Error summary when status=failed."},
					"context_patch": map[string]any{"type": "object", "description": "Structured run context patch."},
				},
				[]string{"goal", "status", "current_step"},
				[]string{"run_id", "waiting_ref", "step_summary", "error", "context_patch"},
			),
		},
	}
}

func (a *Agent) callAutonomousRunCheckpoint(_ context.Context, raw string) (string, error) {
	if a.runs == nil {
		return "", fmt.Errorf("autonomous run manager not configured")
	}
	args, err := readToolArguments(raw)
	if err != nil {
		return "", err
	}
	run, err := a.runs.Checkpoint(AutonomousRunCheckpointInput{
		RunID:        readStringArgument(args, "run_id"),
		Goal:         readStringArgument(args, "goal"),
		Status:       readStringArgument(args, "status"),
		CurrentStep:  readStringArgument(args, "current_step"),
		WaitingRef:   readStringArgument(args, "waiting_ref"),
		StepSummary:  readStringArgument(args, "step_summary"),
		Error:        readStringArgument(args, "error"),
		ContextPatch: readObjectArgument(args, "context_patch"),
	})
	if err != nil {
		return "", err
	}
	a.resumeWaitingRunIfTaskAlreadyTerminal(run)
	return renderAutonomousRunOutput(run), nil
}

func (a *Agent) resumeWaitingRunIfTaskAlreadyTerminal(run AutonomousRun) {
	if a == nil || a.asyncTasks == nil || a.runs == nil {
		return
	}
	if strings.TrimSpace(run.Status) != autonomousRunStatusWaitingAsync {
		return
	}
	taskID := strings.TrimSpace(run.WaitingRef)
	if taskID == "" {
		return
	}
	task, ok := a.asyncTasks.GetLocal(taskID)
	if !ok || !isAsyncTaskTerminal(task.Status) {
		return
	}
	go a.resumeAutonomousRunsForTask(context.Background(), task)
}

func renderAutonomousRunOutput(run AutonomousRun) string {
	data := map[string]any{
		"run_id":       run.ID,
		"goal":         run.Goal,
		"status":       run.Status,
		"current_step": run.CurrentStep,
	}
	if strings.TrimSpace(run.WaitingType) != "" {
		data["waiting_type"] = run.WaitingType
	}
	if strings.TrimSpace(run.WaitingRef) != "" {
		data["waiting_ref"] = run.WaitingRef
	}
	if strings.TrimSpace(run.LastEventType) != "" {
		data["last_event_type"] = run.LastEventType
	}
	if strings.TrimSpace(run.LastEventSummary) != "" {
		data["last_event_summary"] = run.LastEventSummary
	}
	if strings.TrimSpace(run.Error) != "" {
		data["error"] = run.Error
	}
	if len(run.Context) > 0 {
		data["context"] = run.Context
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Sprintf(`{"run_id":"%s","status":"%s","current_step":"%s"}`, run.ID, run.Status, run.CurrentStep)
	}
	return string(raw)
}
