package agent

type AutonomousRunStateStore interface {
	Load() ([]AutonomousRun, error)
	Save(runs []AutonomousRun) error
}
