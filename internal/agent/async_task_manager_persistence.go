package agent

import (
	"context"
	"strconv"
	"strings"
	"time"
)

func (m *AsyncTaskManager) collectResumableA2ATaskIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.collectResumableA2ATaskIDsLocked()
}

func (m *AsyncTaskManager) collectResumableA2ATaskIDsLocked() []string {
	if m.a2a == nil {
		return nil
	}
	out := make([]string, 0, len(m.order))
	for _, taskID := range m.order {
		state := m.tasks[taskID]
		if !isResumableA2AState(state) {
			continue
		}
		out = append(out, taskID)
	}
	return out
}

func (m *AsyncTaskManager) resumeA2ATasks(taskIDs []string) {
	if len(taskIDs) == 0 || m.readA2AProvider() == nil {
		return
	}
	for _, taskID := range taskIDs {
		m.startRecoveredA2AWorker(taskID)
	}
}

func (m *AsyncTaskManager) startRecoveredA2AWorker(taskID string) {
	task, ctx, ok := m.markRecoveredWorkerRunning(taskID)
	if !ok {
		return
	}
	go func() {
		defer m.setWorkerRunning(task.ID, false)
		m.pollA2ATask(ctx, task.ID, task.AgentID, task.RemoteTaskID)
	}()
}

func (m *AsyncTaskManager) markRecoveredWorkerRunning(taskID string) (AsyncTask, context.Context, bool) {
	m.mu.Lock()
	state := m.tasks[taskID]
	if !isResumableA2AState(state) {
		m.mu.Unlock()
		return AsyncTask{}, nil, false
	}
	taskCtx, cancel := ensureTaskContext(state)
	state.ctx = taskCtx
	state.cancel = cancel
	state.workerRunning = true
	state.task.TrackerState = asyncTaskTrackerStateRecovering
	state.task.TrackerReason = asyncTaskTrackerReasonNone
	state.task.UpdatedAt = m.nowFn().Truncate(time.Second)
	m.appendLogLocked(state, "info", "a2a tracking resumed")
	m.persistSnapshotLocked()
	snapshot := cloneAsyncTask(state.task)
	m.mu.Unlock()
	m.emitStatusChange(snapshot)
	return snapshot, taskCtx, true
}

func (m *AsyncTaskManager) setWorkerRunning(taskID string, running bool) {
	m.mu.Lock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.Unlock()
		return
	}
	state.workerRunning = running
	m.persistSnapshotLocked()
	m.mu.Unlock()
}

func ensureTaskContext(state *asyncTaskState) (context.Context, context.CancelFunc) {
	if state == nil {
		return context.Background(), func() {}
	}
	if state.ctx == nil || isContextDone(state.ctx) {
		return context.WithCancel(context.Background())
	}
	return state.ctx, state.cancel
}

func isContextDone(ctx context.Context) bool {
	if ctx == nil {
		return true
	}
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func isResumableA2AState(state *asyncTaskState) bool {
	if state == nil {
		return false
	}
	task := state.task
	if task.TaskType != asyncTaskTypeA2A || isAsyncTaskTerminal(task.Status) {
		return false
	}
	if strings.TrimSpace(task.RemoteTaskID) == "" || state.cancelRequested || state.workerRunning {
		return false
	}
	return true
}

func (m *AsyncTaskManager) persistSnapshotLocked() {
	if m.stateStore == nil {
		return
	}
	snapshot := make([]AsyncTask, 0, len(m.order))
	for _, taskID := range m.order {
		state := m.tasks[taskID]
		if state == nil {
			continue
		}
		snapshot = append(snapshot, cloneAsyncTask(state.task))
	}
	_ = m.stateStore.Save(snapshot)
}

func (m *AsyncTaskManager) loadSnapshotFromStore() error {
	m.mu.RLock()
	store := m.stateStore
	m.mu.RUnlock()
	if store == nil {
		return nil
	}
	items, err := store.Load()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.tasks = make(map[string]*asyncTaskState, len(items))
	m.order = make([]string, 0, len(items))
	m.seq = 0
	for _, item := range items {
		task := normalizeLoadedTask(item, m.nowFn().Truncate(time.Second))
		state := &asyncTaskState{task: task}
		if !isAsyncTaskTerminal(task.Status) {
			state.ctx, state.cancel = context.WithCancel(context.Background())
		}
		state.workerRunning = false
		m.tasks[task.ID] = state
		m.order = append(m.order, task.ID)
		m.seq = maxInt64(m.seq, parseTaskSequence(task.ID))
	}
	m.mu.Unlock()
	return nil
}

func normalizeLoadedTask(task AsyncTask, fallbackNow time.Time) AsyncTask {
	task.ID = strings.TrimSpace(task.ID)
	task.TaskType = strings.TrimSpace(task.TaskType)
	task.Status = strings.TrimSpace(task.Status)
	task.TrackerState = strings.TrimSpace(task.TrackerState)
	task.TrackerReason = strings.TrimSpace(task.TrackerReason)
	task.AgentID = strings.TrimSpace(task.AgentID)
	task.RemoteTaskID = strings.TrimSpace(task.RemoteTaskID)
	if task.Status == "" {
		task.Status = asyncTaskStatusSubmitted
	}
	if task.TrackerState == "" {
		task.TrackerState = asyncTaskTrackerStateIdle
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = fallbackNow
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = fallbackNow
	}
	if len(task.Logs) > maxAsyncTaskLogsRetained {
		task.Logs = append([]AsyncTaskLog(nil), task.Logs[len(task.Logs)-maxAsyncTaskLogsRetained:]...)
	}
	for i := range task.Logs {
		task.Logs[i].Cursor = i + 1
		if task.Logs[i].CreatedAt.IsZero() {
			task.Logs[i].CreatedAt = task.UpdatedAt
		}
	}
	return task
}

func parseTaskSequence(taskID string) int64 {
	idx := strings.LastIndex(taskID, "_")
	if idx <= 0 || idx+1 >= len(taskID) {
		return 0
	}
	seq, err := strconv.ParseInt(taskID[idx+1:], 10, 64)
	if err != nil {
		return 0
	}
	return seq
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
