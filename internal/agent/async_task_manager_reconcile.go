package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (m *AsyncTaskManager) reconcileOnGet(task AsyncTask) (AsyncTask, error) {
	if task.TaskType != asyncTaskTypeA2A || isAsyncTaskTerminal(task.Status) {
		return task, nil
	}
	if strings.TrimSpace(task.RemoteTaskID) == "" {
		return task, nil
	}
	policy := m.readTrackingPolicy()
	now := m.nowFn().Truncate(time.Second)
	if m.shouldSkipReconcile(task, now, policy.MinReconcileInterval) {
		return m.markReconcileDebounced(task.ID), nil
	}
	provider := m.readA2AProvider()
	if provider == nil {
		return AsyncTask{}, fmt.Errorf("a2a provider not configured")
	}
	result, err := provider.GetTask(context.Background(), A2ATaskQuery{
		AgentID: task.AgentID,
		TaskID:  task.RemoteTaskID,
	})
	if err != nil {
		return m.markReconcileError(task.ID, err), nil
	}
	return m.applyReconcileResult(task.ID, result), nil
}

func (m *AsyncTaskManager) shouldSkipReconcile(task AsyncTask, now time.Time, minInterval time.Duration) bool {
	if minInterval <= 0 || task.LastReconciledAt.IsZero() {
		return false
	}
	return now.Sub(task.LastReconciledAt) < minInterval
}

func (m *AsyncTaskManager) markReconcileDebounced(taskID string) AsyncTask {
	m.mu.Lock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.Unlock()
		return AsyncTask{}
	}
	snapshot := cloneAsyncTask(state.task)
	snapshot.ReconcileSkippedByDebounce = true
	snapshot.TrackerReason = asyncTaskTrackerReasonReconcileDebounced
	m.mu.Unlock()
	return snapshot
}

func (m *AsyncTaskManager) markReconcileError(taskID string, err error) AsyncTask {
	m.mu.Lock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.Unlock()
		return AsyncTask{}
	}
	state.task.TrackerState = asyncTaskTrackerStatePaused
	state.task.TrackerReason = asyncTaskTrackerReasonReconcileError
	state.task.UpdatedAt = m.nowFn().Truncate(time.Second)
	state.task.LastReconciledAt = state.task.UpdatedAt
	state.task.NextPollAt = state.task.UpdatedAt.Add(m.a2aPolicy.MaxInterval)
	m.appendLogLocked(state, "warn", "a2a reconcile failed: "+trimRunes(strings.TrimSpace(err.Error()), 200))
	m.persistSnapshotLocked()
	snapshot := cloneAsyncTask(state.task)
	m.mu.Unlock()
	m.emitStatusChange(snapshot)
	m.scheduleA2ARecovery(taskID, snapshot.NextPollAt)
	return snapshot
}

func (m *AsyncTaskManager) applyReconcileResult(taskID string, result A2ATaskResult) AsyncTask {
	if terminal := normalizeA2ATerminalStatus(result.Status); terminal != "" {
		m.finishTask(taskID, terminal, renderA2ATaskResult(result), "")
		task, _ := m.getTaskSnapshot(taskID)
		return task
	}
	if blockedErr := normalizeA2ABlockedStatus(result.Status); blockedErr != "" {
		m.finishTask(taskID, asyncTaskStatusFailed, renderA2ATaskResult(result), blockedErr)
		task, _ := m.getTaskSnapshot(taskID)
		return task
	}
	if !isA2AInProgressStatus(result.Status) {
		m.finishTask(
			taskID,
			asyncTaskStatusFailed,
			renderA2ATaskResult(result),
			"a2a get returned unsupported status: "+strings.TrimSpace(result.Status),
		)
		task, _ := m.getTaskSnapshot(taskID)
		return task
	}
	m.mu.Lock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.Unlock()
		return AsyncTask{}
	}
	now := m.nowFn().Truncate(time.Second)
	state.task.Status = asyncTaskStatusWorking
	state.task.TrackerState = asyncTaskTrackerStateActive
	state.task.TrackerReason = asyncTaskTrackerReasonNone
	state.task.ConsecutiveErrors = 0
	state.task.LastReconciledAt = now
	state.task.UpdatedAt = now
	state.task.ProgressSummary = buildA2AProgressSummary(result)
	policy := m.a2aPolicy
	state.task.NextPollAt = now.Add(policy.InitialInterval)
	logMessage := "a2a reconcile status: " + strings.TrimSpace(result.Status)
	if text := strings.TrimSpace(state.task.ProgressSummary); text != "" {
		logMessage += " | " + text
	}
	m.appendLogLocked(state, "info", logMessage)
	m.persistSnapshotLocked()
	snapshot := cloneAsyncTask(state.task)
	m.mu.Unlock()
	m.emitStatusChange(snapshot)
	return snapshot
}

func (m *AsyncTaskManager) getTaskSnapshot(taskID string) (AsyncTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.tasks[taskID]
	if state == nil {
		return AsyncTask{}, fmt.Errorf("task %q not found", taskID)
	}
	return cloneAsyncTask(state.task), nil
}

func (m *AsyncTaskManager) readTrackingPolicy() A2ATrackingPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.a2aPolicy
}
