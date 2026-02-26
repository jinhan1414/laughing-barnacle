package agent

type A2ASendRequest struct {
	AgentID   string
	Message   string
	SessionID string
	Metadata  map[string]any
}

type A2ATaskQuery struct {
	AgentID string
	TaskID  string
}

type A2ATaskResult struct {
	AgentID   string
	TaskID    string
	Status    string
	Message   string
	Artifacts []string
}
