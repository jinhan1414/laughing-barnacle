package agent

import "fmt"

func (a *Agent) ResetAutonomousRuns() error {
	if a == nil || a.runs == nil {
		return fmt.Errorf("autonomous run manager not configured")
	}
	return a.runs.Reset()
}
