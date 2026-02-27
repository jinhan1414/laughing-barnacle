package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"laughing-barnacle/internal/llm"
)

func (m *AsyncTaskManager) markTaskWorking(taskID string) (AsyncTask, context.Context, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.tasks[taskID]
	if state == nil {
		return AsyncTask{}, nil, fmt.Errorf("task %q not found", taskID)
	}
	state.task.Status = asyncTaskStatusWorking
	state.task.UpdatedAt = m.nowFn().Truncate(time.Second)
	m.appendLogLocked(state, "info", "task started")
	snapshot := cloneAsyncTask(state.task)
	go m.emitStatusChange(snapshot)
	return snapshot, state.ctx, nil
}

func (m *AsyncTaskManager) finishTask(taskID, status, result, errText string) {
	m.mu.Lock()
	state := m.tasks[taskID]
	if state == nil {
		m.mu.Unlock()
		return
	}
	state.task.Status = status
	state.task.Result = strings.TrimSpace(result)
	state.task.Error = strings.TrimSpace(errText)
	state.task.UpdatedAt = m.nowFn().Truncate(time.Second)
	if state.task.Error != "" {
		m.appendLogLocked(state, "error", state.task.Error)
	} else {
		m.appendLogLocked(state, "info", "task finished: "+status)
	}
	snapshot := cloneAsyncTask(state.task)
	m.mu.Unlock()

	m.emitStatusChange(snapshot)
	m.emitCompletion(snapshot)
}

func (m *AsyncTaskManager) runGenericTask(ctx context.Context, task AsyncTask) {
	if m.llm == nil {
		m.finishTask(task.ID, asyncTaskStatusFailed, "", "llm client unavailable")
		return
	}
	resp, err := m.llm.Chat(ctx, llm.ChatRequest{
		Purpose: "async_task_generic",
		Model:   m.model,
		Messages: []llm.Message{
			{Role: "system", Content: "你是数字分身后台任务执行器。直接输出任务结果正文。"},
			{Role: "user", Content: strings.TrimSpace(task.Request)},
		},
		Temperature: 0.2,
	})
	if err != nil {
		m.finishTask(task.ID, asyncTaskStatusFailed, "", "run generic async task failed: "+err.Error())
		return
	}
	content := strings.TrimSpace(resp.Content)
	if content == "" {
		content = "(空结果)"
	}
	m.finishTask(task.ID, asyncTaskStatusSucceeded, content, "")
}

func (m *AsyncTaskManager) emitStatusChange(task AsyncTask) {
	m.mu.RLock()
	hook := m.onStat
	m.mu.RUnlock()
	if hook != nil {
		hook(task)
	}
}

func (m *AsyncTaskManager) emitCompletion(task AsyncTask) {
	if !task.NotifyOnFinish {
		return
	}
	if !isAsyncTaskTerminal(task.Status) {
		return
	}
	m.mu.RLock()
	hook := m.onDone
	m.mu.RUnlock()
	if hook == nil {
		return
	}
	go hook(context.Background(), task)
}

func (m *AsyncTaskManager) appendLogLocked(state *asyncTaskState, level, message string) {
	if state == nil {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	next := AsyncTaskLog{
		Cursor:    len(state.task.Logs) + 1,
		CreatedAt: m.nowFn().Truncate(time.Second),
		Level:     strings.TrimSpace(level),
		Message:   trimRunes(message, 400),
	}
	state.task.Logs = append(state.task.Logs, next)
	if len(state.task.Logs) > maxAsyncTaskLogsRetained {
		state.task.Logs = append([]AsyncTaskLog(nil), state.task.Logs[len(state.task.Logs)-maxAsyncTaskLogsRetained:]...)
	}
}

func (m *AsyncTaskManager) trimTaskEntriesLocked() {
	if len(m.order) <= maxAsyncTaskEntries {
		return
	}
	drop := len(m.order) - maxAsyncTaskEntries
	for i := 0; i < drop; i++ {
		delete(m.tasks, m.order[i])
	}
	m.order = append([]string(nil), m.order[drop:]...)
}

func sliceTaskLogs(logs []AsyncTaskLog, cursor, limit int) []AsyncTaskLog {
	if len(logs) == 0 || limit == 0 {
		return nil
	}
	out := make([]AsyncTaskLog, 0, limit)
	for _, item := range logs {
		if item.Cursor <= cursor {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func cloneAsyncTask(in AsyncTask) AsyncTask {
	out := in
	out.Metadata = cloneAnyMap(in.Metadata)
	if len(in.Logs) > 0 {
		out.Logs = append([]AsyncTaskLog(nil), in.Logs...)
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
