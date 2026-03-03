package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"laughing-barnacle/internal/conversation"
)

const autonomousRunResumeTimeout = 2 * time.Minute

func (a *Agent) onAutonomousRunStatusChanged(run AutonomousRun) {
	if a == nil || a.store == nil {
		return
	}
	parts := []string{
		"run_id=" + safeOrEmpty(run.ID),
		"status=" + safeOrEmpty(run.Status),
		"step=" + safeOrEmpty(run.CurrentStep),
		"goal=" + trimRunes(safeOrEmpty(run.Goal), 80),
	}
	if text := strings.TrimSpace(run.WaitingType); text != "" {
		parts = append(parts, "waiting="+text)
	}
	if text := strings.TrimSpace(run.WaitingRef); text != "" {
		parts = append(parts, "waiting_ref="+text)
	}
	if text := strings.TrimSpace(run.Error); text != "" {
		parts = append(parts, "error="+trimRunes(text, 160))
	}
	a.store.AppendEvent("autonomous_run_status", strings.Join(parts, " | "))
}

func (a *Agent) resumeAutonomousRunsForTask(parentCtx context.Context, task AsyncTask) {
	if a == nil || a.runs == nil {
		return
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	if parentCtx.Err() != nil {
		return
	}
	runs := a.runs.MatchWaitingAsyncTask(task.ID)
	for _, run := range runs {
		if parentCtx.Err() != nil {
			return
		}
		a.resumeAutonomousRun(parentCtx, task, run)
	}
}

func (a *Agent) resumeAutonomousRun(parentCtx context.Context, task AsyncTask, run AutonomousRun) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, autonomousRunResumeTimeout)
	defer cancel()
	if ctx.Err() != nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	current, err := a.runs.Get(run.ID)
	if err != nil {
		return
	}
	if current.Status != autonomousRunStatusWaitingAsync || strings.TrimSpace(current.WaitingRef) != strings.TrimSpace(task.ID) {
		return
	}
	beforeStepCount := len(current.Steps)
	resumeInput := buildAutonomousRunResumeInput(current, task)
	reply, toolCalls, usage, err := a.generateReply(ctx, []conversation.Message{
		{
			Role:      "user",
			Content:   resumeInput,
			CreatedAt: a.nowFn(),
		},
	})
	if err != nil {
		a.failAutonomousRunLocked(current, "resume run failed: "+err.Error())
		return
	}
	updated, err := a.runs.Get(current.ID)
	if err != nil || len(updated.Steps) <= beforeStepCount || !containsCheckpointToolCall(toolCalls, current.ID) {
		a.failAutonomousRunLocked(current, "resume run missing checkpoint")
		return
	}
	reply = sanitizeLLMReply(reply)
	if strings.TrimSpace(reply) != "" {
		a.store.AppendAssistant(reply, usage)
	}
}

func (a *Agent) failAutonomousRunLocked(run AutonomousRun, reason string) {
	if a == nil || a.runs == nil {
		return
	}
	_, _ = a.runs.Checkpoint(AutonomousRunCheckpointInput{
		RunID:       run.ID,
		Goal:        run.Goal,
		Status:      autonomousRunStatusFailed,
		CurrentStep: run.CurrentStep,
		StepSummary: "自动恢复失败",
		Error:       trimRunes(strings.TrimSpace(reason), 180),
	})
}

func buildAutonomousRunResumeInput(run AutonomousRun, task AsyncTask) string {
	payload := map[string]any{
		"run": map[string]any{
			"run_id":       run.ID,
			"goal":         run.Goal,
			"status":       run.Status,
			"current_step": run.CurrentStep,
			"waiting_type": run.WaitingType,
			"waiting_ref":  run.WaitingRef,
			"last_event":   run.LastEventType,
			"last_summary": run.LastEventSummary,
			"error":        run.Error,
			"context":      run.Context,
			"recent_steps": run.Steps,
			"created_at":   run.CreatedAt.Format(time.RFC3339),
			"updated_at":   run.UpdatedAt.Format(time.RFC3339),
		},
		"event": map[string]any{
			"type":           "async_task.completed",
			"task_id":        task.ID,
			"status":         task.Status,
			"task_type":      task.TaskType,
			"request":        task.Request,
			"result":         task.Result,
			"error":          task.Error,
			"remote_task_id": task.RemoteTaskID,
		},
	}
	raw, _ := json.Marshal(payload)
	return strings.TrimSpace(
		"你正在恢复一个自主运行中的多步任务，不是在处理新的用户提问。\n" +
			"硬约束：你必须先基于事件判断下一步，再在结束前调用 autonomous_run__checkpoint，且必须复用同一个 run_id，并继续携带同一个 goal。\n" +
			"如果需要继续后台执行，先调用 async_task__submit，再调用 autonomous_run__checkpoint 把状态写成 waiting_async 并记录 waiting_ref=task_id。\n" +
			"如果需要等待用户补充信息，写成 waiting_human。\n" +
			"如果任务已经完成，写成 completed。\n" +
			"如果无法继续，写成 failed 并给出明确错误。\n" +
			"下面是本次恢复执行的结构化上下文包：\n" + string(raw),
	)
}

func containsCheckpointToolCall(toolCalls []conversation.ToolCall, runID string) bool {
	for _, call := range toolCalls {
		if strings.TrimSpace(call.Name) != builtinAutonomousRunCheckpointToolName {
			continue
		}
		if strings.TrimSpace(runID) == "" {
			return true
		}
		if strings.Contains(call.Result, `"`+runID+`"`) {
			return true
		}
	}
	return false
}
