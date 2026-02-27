package agent

import (
	"context"
	"fmt"
	"laughing-barnacle/internal/llm"
	"strconv"
	"strings"
	"sync"
	"time"
)

type asyncTaskState struct {
	task            AsyncTask
	ctx             context.Context
	cancel          context.CancelFunc
	cancelRequested bool
}

type AsyncTaskManager struct {
	mu sync.RWMutex

	nowFn  func() time.Time
	llm    llm.Client
	model  string
	a2a    A2AProvider
	seq    int64
	order  []string
	tasks  map[string]*asyncTaskState
	onStat func(AsyncTask)
	onDone func(context.Context, AsyncTask)
}

func newAsyncTaskManager(llmClient llm.Client, model string, nowFn func() time.Time) *AsyncTaskManager {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &AsyncTaskManager{
		nowFn: nowFn,
		llm:   llmClient,
		model: strings.TrimSpace(model),
		tasks: make(map[string]*asyncTaskState),
		order: make([]string, 0, 16),
	}
}

func (m *AsyncTaskManager) SetA2AProvider(provider A2AProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.a2a = provider
}

func (m *AsyncTaskManager) SetHooks(onStatus func(AsyncTask), onDone func(context.Context, AsyncTask)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onStat = onStatus
	m.onDone = onDone
}

func (m *AsyncTaskManager) Submit(ctx context.Context, input AsyncTaskSubmitInput) (AsyncTask, error) {
	taskType, err := normalizeAsyncTaskType(input.TaskType)
	if err != nil {
		return AsyncTask{}, err
	}
	input.Request = strings.TrimSpace(input.Request)
	if input.Request == "" {
		return AsyncTask{}, fmt.Errorf("request is required")
	}
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.AgentInput = strings.TrimSpace(input.AgentInput)
	input.DedupeKey = strings.TrimSpace(input.DedupeKey)
	if taskType == asyncTaskTypeA2A && (input.AgentID == "" || input.AgentInput == "") {
		return AsyncTask{}, fmt.Errorf("agent_id and agent_input are required when task_type=a2a")
	}
	taskID, created := m.createOrReuseTask(input, taskType)
	if !created {
		return m.Get(AsyncTaskGetInput{TaskID: taskID})
	}
	go m.runTask(taskID)
	return m.Get(AsyncTaskGetInput{TaskID: taskID})
}

func (m *AsyncTaskManager) createOrReuseTask(input AsyncTaskSubmitInput, taskType string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if input.DedupeKey != "" {
		for _, id := range m.order {
			state := m.tasks[id]
			if state == nil || state.task.DedupeKey != input.DedupeKey || isAsyncTaskTerminal(state.task.Status) {
				continue
			}
			return id, false
		}
	}

	now := m.nowFn().Truncate(time.Second)
	taskID := m.nextTaskIDLocked(now)
	taskCtx, cancel := context.WithCancel(context.Background())
	state := &asyncTaskState{
		task: AsyncTask{
			ID:             taskID,
			TaskType:       taskType,
			Status:         asyncTaskStatusSubmitted,
			Request:        input.Request,
			AgentID:        input.AgentID,
			AgentInput:     input.AgentInput,
			DedupeKey:      input.DedupeKey,
			NotifyOnFinish: input.NotifyOnFinish,
			Metadata:       cloneAnyMap(input.Metadata),
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		ctx:    taskCtx,
		cancel: cancel,
	}
	m.appendLogLocked(state, "info", "task accepted")
	m.tasks[taskID] = state
	m.order = append(m.order, taskID)
	m.trimTaskEntriesLocked()
	go m.emitStatusChange(cloneAsyncTask(state.task))
	return taskID, true
}

func (m *AsyncTaskManager) Get(input AsyncTaskGetInput) (AsyncTask, error) {
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return AsyncTask{}, fmt.Errorf("task_id is required")
	}
	limit, err := normalizeAsyncTaskLogLimit(input.LogLimit)
	if err != nil {
		return AsyncTask{}, err
	}
	m.mu.RLock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.RUnlock()
		return AsyncTask{}, fmt.Errorf("task %q not found", taskID)
	}
	task := cloneAsyncTask(state.task)
	m.mu.RUnlock()
	if !input.IncludeLogs {
		task.Logs = nil
		return task, nil
	}
	task.Logs = sliceTaskLogs(task.Logs, input.LogCursor, limit)
	return task, nil
}

func (m *AsyncTaskManager) Cancel(ctx context.Context, input AsyncTaskCancelInput) (AsyncTask, error) {
	_ = ctx
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return AsyncTask{}, fmt.Errorf("task_id is required")
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "user_cancel"
	}
	task, err := m.markCancelRequested(taskID, reason)
	if err != nil {
		return AsyncTask{}, err
	}
	return task, nil
}

func (m *AsyncTaskManager) markCancelRequested(taskID, reason string) (AsyncTask, error) {
	m.mu.Lock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.Unlock()
		return AsyncTask{}, fmt.Errorf("task %q not found", taskID)
	}
	if isAsyncTaskTerminal(state.task.Status) {
		task := cloneAsyncTask(state.task)
		m.mu.Unlock()
		return task, fmt.Errorf("task %q is already terminal", taskID)
	}
	state.cancelRequested = true
	m.appendLogLocked(state, "warn", "cancel requested: "+reason)
	state.task.UpdatedAt = m.nowFn().Truncate(time.Second)
	snapshot := cloneAsyncTask(state.task)
	cancel := state.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	go m.emitStatusChange(snapshot)
	return snapshot, nil
}

func (m *AsyncTaskManager) ListTasks() []AsyncTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]AsyncTask, 0, len(m.order))
	for i := len(m.order) - 1; i >= 0; i-- {
		if state := m.tasks[m.order[i]]; state != nil {
			out = append(out, cloneAsyncTask(state.task))
		}
	}
	return out
}

func (m *AsyncTaskManager) ListIndexLines(limit int, now time.Time) []string {
	items := m.ListTasks()
	out := make([]string, 0, len(items))
	for _, task := range items {
		if !includeTaskInIndex(task, now) {
			continue
		}
		out = append(out, fmt.Sprintf(
			"task_id=%s | type=%s | status=%s | updated_at=%s | brief=%s",
			task.ID, task.TaskType, task.Status, task.UpdatedAt.Local().Format("2006-01-02 15:04:05"),
			trimRunes(task.Request, 80),
		))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func includeTaskInIndex(task AsyncTask, now time.Time) bool {
	if isAsyncTaskTerminal(task.Status) {
		return task.CreatedAt.Local().Year() == now.Local().Year() &&
			task.CreatedAt.Local().Month() == now.Local().Month() &&
			task.CreatedAt.Local().Day() == now.Local().Day()
	}
	return true
}

func (m *AsyncTaskManager) nextTaskIDLocked(now time.Time) string {
	m.seq++
	return "async_" + now.Format("20060102_150405") + "_" + strconv.FormatInt(m.seq, 10)
}

func (m *AsyncTaskManager) runTask(taskID string) {
	task, ctx, err := m.markTaskWorking(taskID)
	if err != nil {
		return
	}
	switch task.TaskType {
	case asyncTaskTypeA2A:
		m.runA2ATask(ctx, task)
	default:
		m.runGenericTask(ctx, task)
	}
}
