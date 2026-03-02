package agent

import "fmt"

func (a *Agent) BindAutonomousRunStateStore(store AutonomousRunStateStore) error {
	if a == nil || a.runs == nil {
		return fmt.Errorf("autonomous run manager not configured")
	}
	return a.runs.BindStateStore(store)
}

func (a *Agent) ListAutonomousRuns() []AutonomousRun {
	if a == nil || a.runs == nil {
		return nil
	}
	return a.runs.ListRuns()
}

func (a *Agent) GetAutonomousRun(runID string) (AutonomousRun, error) {
	if a == nil || a.runs == nil {
		return AutonomousRun{}, fmt.Errorf("autonomous run manager not configured")
	}
	return a.runs.Get(runID)
}
