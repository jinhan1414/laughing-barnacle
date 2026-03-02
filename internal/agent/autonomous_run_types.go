package agent

import "time"

const (
	autonomousRunStatusRunning      = "running"
	autonomousRunStatusWaitingAsync = "waiting_async"
	autonomousRunStatusWaitingHuman = "waiting_human"
	autonomousRunStatusCompleted    = "completed"
	autonomousRunStatusFailed       = "failed"

	autonomousRunWaitingTypeAsync = "async_task"
	autonomousRunWaitingTypeHuman = "human_reply"
)

const (
	maxAutonomousRunsRetained  = 80
	maxAutonomousRunSteps      = 40
	maxAutonomousRunIndexLines = 12
)

type AutonomousRun struct {
	ID               string         `json:"id"`
	Goal             string         `json:"goal"`
	Status           string         `json:"status"`
	CurrentStep      string         `json:"current_step"`
	WaitingType      string         `json:"waiting_type,omitempty"`
	WaitingRef       string         `json:"waiting_ref,omitempty"`
	LastEventType    string         `json:"last_event_type,omitempty"`
	LastEventSummary string         `json:"last_event_summary,omitempty"`
	Error            string         `json:"error,omitempty"`
	Context          map[string]any `json:"context,omitempty"`
	Steps            []RunStep      `json:"steps,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type RunStep struct {
	Seq         int       `json:"seq"`
	Step        string    `json:"step"`
	Status      string    `json:"status"`
	Summary     string    `json:"summary,omitempty"`
	WaitingType string    `json:"waiting_type,omitempty"`
	WaitingRef  string    `json:"waiting_ref,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type AutonomousRunCheckpointInput struct {
	RunID         string
	Goal          string
	Status        string
	CurrentStep   string
	WaitingType   string
	WaitingRef    string
	StepSummary   string
	Error         string
	ContextPatch  map[string]any
	LastEventType string
	LastEventText string
}

func isAutonomousRunTerminal(status string) bool {
	return status == autonomousRunStatusCompleted || status == autonomousRunStatusFailed
}
