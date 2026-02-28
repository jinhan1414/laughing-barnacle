package agent

import "context"

func (m *AsyncTaskManager) Reset() error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	if m.stateStore != nil {
		if err := m.stateStore.Save(nil); err != nil {
			m.mu.Unlock()
			return err
		}
	}
	cancels := collectTaskCancels(m.tasks, m.order)
	m.tasks = make(map[string]*asyncTaskState)
	m.order = nil
	m.seq = 0
	m.mu.Unlock()

	cancelTaskContexts(cancels)
	return nil
}

func collectTaskCancels(tasks map[string]*asyncTaskState, order []string) []context.CancelFunc {
	cancels := make([]context.CancelFunc, 0, len(order))
	for _, taskID := range order {
		state := tasks[taskID]
		if state == nil {
			continue
		}
		state.cancelRequested = true
		if state.cancel != nil {
			cancels = append(cancels, state.cancel)
		}
	}
	return cancels
}

func cancelTaskContexts(cancels []context.CancelFunc) {
	for _, cancel := range cancels {
		cancel()
	}
}
