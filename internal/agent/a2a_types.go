package agent

import "time"

type A2ARegisterRequest struct {
	AgentCardURL  string
	AgentCardJSON string
	Alias         string
	Description   string
	Endpoint      string
	AuthToken     string
	Enabled       bool
}

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

type A2AAgentDetail struct {
	ID           string
	Name         string
	Description  string
	Endpoint     string
	AgentCardURL string
	Enabled      bool
	UpdatedAt    time.Time
}
