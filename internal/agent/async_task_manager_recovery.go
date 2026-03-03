package agent

import (
	"strings"
	"time"
)

func (m *AsyncTaskManager) scheduleA2ARecovery(taskID string, retryAt time.Time) {
	if strings.TrimSpace(taskID) == "" {
		return
	}
	delay := time.Until(retryAt)
	if retryAt.IsZero() || delay < 0 {
		delay = 0
	}
	time.AfterFunc(delay, func() {
		m.startRecoveredA2AWorker(taskID)
	})
}
