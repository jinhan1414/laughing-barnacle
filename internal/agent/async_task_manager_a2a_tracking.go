package agent

import (
	"strings"
	"time"
)

func (m *AsyncTaskManager) handlePollingTerminal(taskID string, result A2ATaskResult) bool {
	if terminal := normalizeA2ATerminalStatus(result.Status); terminal != "" {
		m.finishTask(taskID, terminal, renderA2ATaskResult(result), "")
		return true
	}
	if blockedErr := normalizeA2ABlockedStatus(result.Status); blockedErr != "" {
		m.finishTask(taskID, asyncTaskStatusFailed, renderA2ATaskResult(result), blockedErr)
		return true
	}
	if isA2AInProgressStatus(result.Status) {
		return false
	}
	m.finishTask(
		taskID,
		asyncTaskStatusFailed,
		renderA2ATaskResult(result),
		"a2a get returned unsupported status: "+strings.TrimSpace(result.Status),
	)
	return true
}

func (m *AsyncTaskManager) handlePollingError(taskID string, pollErr error, pollInterval time.Duration, policy A2ATrackingPolicy) (time.Duration, bool) {
	m.mu.Lock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.Unlock()
		return pollInterval, true
	}
	state.task.ConsecutiveErrors++
	state.task.TrackerState = asyncTaskTrackerStateRecovering
	state.task.TrackerReason = asyncTaskTrackerReasonNone
	state.task.UpdatedAt = m.nowFn().Truncate(time.Second)
	if state.task.ConsecutiveErrors >= policy.MaxConsecutiveErrors {
		retryAt := state.task.UpdatedAt.Add(policy.MaxInterval)
		state.task.TrackerState = asyncTaskTrackerStatePaused
		state.task.TrackerReason = asyncTaskTrackerReasonConsecutiveErrorsReached
		state.task.NextPollAt = retryAt
		m.appendLogLocked(state, "warn", "a2a tracking paused after polling errors: "+trimRunes(strings.TrimSpace(pollErr.Error()), 200))
		m.persistSnapshotLocked()
		snapshot := cloneAsyncTask(state.task)
		state.workerRunning = false
		m.mu.Unlock()
		m.emitStatusChange(snapshot)
		m.scheduleA2ARecovery(taskID, retryAt)
		return pollInterval, true
	}
	nextInterval := nextBackoffInterval(pollInterval, policy)
	state.task.NextPollAt = state.task.UpdatedAt.Add(nextInterval)
	m.appendLogLocked(state, "warn", "a2a get failed, will retry: "+trimRunes(strings.TrimSpace(pollErr.Error()), 200))
	m.persistSnapshotLocked()
	snapshot := cloneAsyncTask(state.task)
	m.mu.Unlock()
	m.emitStatusChange(snapshot)
	return nextInterval, false
}

func (m *AsyncTaskManager) updateInProgressTracking(taskID string, result A2ATaskResult, windowStartedAt time.Time, policy A2ATrackingPolicy) (time.Time, bool) {
	progress := buildA2AProgressSummary(result)
	logMessage := "a2a status: " + strings.TrimSpace(result.Status)
	if progress != "" {
		logMessage += " | " + progress
	}
	m.updatePollingLog(taskID, logMessage)
	m.mu.Lock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.Unlock()
		return windowStartedAt, true
	}
	now := m.nowFn().Truncate(time.Second)
	state.task.LastReconciledAt = now
	state.task.ProgressSummary = progress
	if now.Sub(windowStartedAt) < policy.MaxTrackingDuration {
		m.persistSnapshotLocked()
		m.mu.Unlock()
		return windowStartedAt, false
	}
	if state.task.TrackingRenewals < policy.MaxTrackingRenewals {
		state.task.TrackingRenewals++
		state.task.LastRenewedAt = now
		state.task.TrackerState = asyncTaskTrackerStateActive
		state.task.TrackerReason = asyncTaskTrackerReasonTrackingRenewed
		m.appendLogLocked(state, "info", "tracking_renewed")
		m.persistSnapshotLocked()
		snapshot := cloneAsyncTask(state.task)
		m.mu.Unlock()
		m.emitStatusChange(snapshot)
		return now, false
	}
	state.task.TrackerState = asyncTaskTrackerStatePaused
	state.task.TrackerReason = asyncTaskTrackerReasonWindowExhausted
	state.task.NextPollAt = now.Add(policy.MaxInterval)
	state.task.UpdatedAt = now
	m.appendLogLocked(state, "warn", "a2a tracking paused: tracking_window_exhausted")
	m.persistSnapshotLocked()
	snapshot := cloneAsyncTask(state.task)
	state.workerRunning = false
	m.mu.Unlock()
	m.emitStatusChange(snapshot)
	m.scheduleA2ARecovery(taskID, snapshot.NextPollAt)
	return windowStartedAt, true
}

func (m *AsyncTaskManager) pauseTracking(taskID, message, reason string) {
	m.mu.Lock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.Unlock()
		return
	}
	state.task.TrackerState = asyncTaskTrackerStatePaused
	state.task.TrackerReason = strings.TrimSpace(reason)
	state.task.NextPollAt = m.nowFn().Truncate(time.Second).Add(m.a2aPolicy.MaxInterval)
	state.task.UpdatedAt = m.nowFn().Truncate(time.Second)
	m.appendLogLocked(state, "warn", trimRunes(strings.TrimSpace(message), 240))
	m.persistSnapshotLocked()
	snapshot := cloneAsyncTask(state.task)
	state.workerRunning = false
	m.mu.Unlock()
	m.emitStatusChange(snapshot)
	m.scheduleA2ARecovery(taskID, snapshot.NextPollAt)
}
