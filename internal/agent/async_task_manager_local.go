package agent

import "strings"

func (m *AsyncTaskManager) GetLocal(taskID string) (AsyncTask, bool) {
	if m == nil {
		return AsyncTask{}, false
	}
	normalizedID := strings.TrimSpace(taskID)
	if normalizedID == "" {
		return AsyncTask{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.tasks[normalizedID]
	if state == nil {
		return AsyncTask{}, false
	}
	return cloneAsyncTask(state.task), true
}
