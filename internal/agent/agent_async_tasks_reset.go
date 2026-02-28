package agent

import "fmt"

func (a *Agent) ResetAsyncTasks() error {
	if a == nil || a.asyncTasks == nil {
		return fmt.Errorf("async task manager not configured")
	}
	return a.asyncTasks.Reset()
}
