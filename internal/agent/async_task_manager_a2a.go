package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	asyncTaskA2APollInterval   = 2 * time.Second
	asyncTaskA2AMaxPollWindows = 45
)

func (m *AsyncTaskManager) runA2ATask(ctx context.Context, task AsyncTask) {
	provider := m.readA2AProvider()
	if provider == nil {
		m.finishTask(task.ID, asyncTaskStatusFailed, "", "a2a provider not configured")
		return
	}

	sendResult, err := provider.Send(ctx, A2ASendRequest{
		AgentID: task.AgentID,
		Message: task.AgentInput,
		Metadata: map[string]any{
			"async_task_id": task.ID,
		},
	})
	if err != nil {
		m.finishTask(task.ID, asyncTaskStatusFailed, "", "a2a send failed: "+err.Error())
		return
	}
	m.updateRemoteTask(task.ID, sendResult.AgentID, sendResult.TaskID, "a2a send accepted")

	if terminal := normalizeA2ATerminalStatus(sendResult.Status); terminal != "" {
		m.finishTask(task.ID, terminal, renderA2ATaskResult(sendResult), "")
		return
	}
	if blockedErr := normalizeA2ABlockedStatus(sendResult.Status); blockedErr != "" {
		m.finishTask(task.ID, asyncTaskStatusFailed, renderA2ATaskResult(sendResult), blockedErr)
		return
	}
	if !isA2AInProgressStatus(sendResult.Status) {
		m.finishTask(
			task.ID,
			asyncTaskStatusFailed,
			renderA2ATaskResult(sendResult),
			"a2a send returned unsupported status: "+strings.TrimSpace(sendResult.Status),
		)
		return
	}
	m.pollA2ATask(ctx, task.ID, sendResult.AgentID, sendResult.TaskID)
}

func (m *AsyncTaskManager) pollA2ATask(ctx context.Context, taskID, agentID, remoteTaskID string) {
	provider := m.readA2AProvider()
	if provider == nil {
		m.finishTask(taskID, asyncTaskStatusFailed, "", "a2a provider not configured")
		return
	}

	for i := 0; i < asyncTaskA2AMaxPollWindows; i++ {
		if err := m.waitPollWindow(ctx); err != nil {
			m.handleCanceledA2ATask(taskID, agentID, remoteTaskID, err)
			return
		}
		result, err := provider.GetTask(ctx, A2ATaskQuery{AgentID: agentID, TaskID: remoteTaskID})
		if err != nil {
			m.finishTask(taskID, asyncTaskStatusFailed, "", "a2a get failed: "+err.Error())
			return
		}
		if terminal := normalizeA2ATerminalStatus(result.Status); terminal != "" {
			m.finishTask(taskID, terminal, renderA2ATaskResult(result), "")
			return
		}
		if blockedErr := normalizeA2ABlockedStatus(result.Status); blockedErr != "" {
			m.finishTask(taskID, asyncTaskStatusFailed, renderA2ATaskResult(result), blockedErr)
			return
		}
		if !isA2AInProgressStatus(result.Status) {
			m.finishTask(
				taskID,
				asyncTaskStatusFailed,
				renderA2ATaskResult(result),
				"a2a get returned unsupported status: "+strings.TrimSpace(result.Status),
			)
			return
		}
		m.updatePollingLog(taskID, "a2a status: "+strings.TrimSpace(result.Status))
	}
	m.finishTask(taskID, asyncTaskStatusFailed, "", "a2a tracking timeout")
}

func (m *AsyncTaskManager) handleCanceledA2ATask(taskID, agentID, remoteTaskID string, runErr error) {
	if !m.isCancelRequested(taskID) {
		m.finishTask(taskID, asyncTaskStatusFailed, "", "a2a tracking interrupted: "+runErr.Error())
		return
	}
	provider := m.readA2AProvider()
	if provider != nil && strings.TrimSpace(remoteTaskID) != "" {
		_, _ = provider.CancelTask(context.Background(), A2ATaskQuery{AgentID: agentID, TaskID: remoteTaskID})
	}
	m.finishTask(taskID, asyncTaskStatusCanceled, "", "")
}

func (m *AsyncTaskManager) waitPollWindow(ctx context.Context) error {
	timer := time.NewTimer(asyncTaskA2APollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (m *AsyncTaskManager) readA2AProvider() A2AProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.a2a
}

func (m *AsyncTaskManager) updateRemoteTask(taskID, agentID, remoteTaskID, message string) {
	m.mu.Lock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.Unlock()
		return
	}
	if strings.TrimSpace(agentID) != "" {
		state.task.AgentID = strings.TrimSpace(agentID)
	}
	state.task.RemoteTaskID = strings.TrimSpace(remoteTaskID)
	state.task.UpdatedAt = m.nowFn().Truncate(time.Second)
	m.appendLogLocked(state, "info", message)
	snapshot := cloneAsyncTask(state.task)
	m.mu.Unlock()
	m.emitStatusChange(snapshot)
}

func (m *AsyncTaskManager) updatePollingLog(taskID, message string) {
	m.mu.Lock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.Unlock()
		return
	}
	state.task.UpdatedAt = m.nowFn().Truncate(time.Second)
	m.appendLogLocked(state, "info", trimRunes(message, 200))
	m.mu.Unlock()
}

func (m *AsyncTaskManager) isCancelRequested(taskID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.tasks[taskID]
	return state != nil && state.cancelRequested
}

func renderAsyncTaskOutput(task AsyncTask, logs []AsyncTaskLog) string {
	var b strings.Builder
	b.WriteString("async_task_id: " + safeOrEmpty(task.ID) + "\n")
	b.WriteString("status: " + safeOrEmpty(task.Status) + "\n")
	b.WriteString("task_type: " + safeOrEmpty(task.TaskType) + "\n")
	if text := strings.TrimSpace(task.AgentID); text != "" {
		b.WriteString("agent_id: " + text + "\n")
	}
	if text := strings.TrimSpace(task.RemoteTaskID); text != "" {
		b.WriteString("remote_task_id: " + text + "\n")
	}
	if text := strings.TrimSpace(task.Result); text != "" {
		b.WriteString("result: " + trimRunes(text, 220) + "\n")
	}
	if text := strings.TrimSpace(task.Error); text != "" {
		b.WriteString("error: " + trimRunes(text, 220) + "\n")
	}
	for _, item := range logs {
		b.WriteString(fmt.Sprintf("log[%d]: %s | %s | %s\n",
			item.Cursor,
			item.CreatedAt.Local().Format("2006-01-02 15:04:05"),
			safeOrEmpty(item.Level),
			safeOrEmpty(item.Message),
		))
	}
	return strings.TrimSpace(b.String())
}
