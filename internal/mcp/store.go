package mcp

import (
	"fmt"
	"laughing-barnacle/internal/agentprompt"
	"laughing-barnacle/internal/scheduler"
	"regexp"
	"strings"
	"sync"
	"time"
)

var serviceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

const (
	ServiceTransportStreamableHTTP = "streamable_http"
	ServiceTransportSSE            = "sse"
	ServiceTransportStdio          = "stdio"
	autoSkillIDPrefix              = "auto-skill-"
	maxAutoSkillsRetained          = 24
	maxAutoSkillNameRunes          = 24
	maxAutoSkillPromptRunes        = 180
	maxTaskRunMessageRunes         = 240
	maxChatGreetingContentRunes    = 240
)

type Service struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Endpoint   string             `json:"endpoint"`
	Command    string             `json:"command,omitempty"`
	Args       []string           `json:"args,omitempty"`
	Transport  string             `json:"transport,omitempty"`
	AuthToken  string             `json:"auth_token,omitempty"`
	Enabled    bool               `json:"enabled"`
	ToolStates []ServiceToolState `json:"tool_states,omitempty"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

type A2AAgent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Endpoint     string    `json:"endpoint"`
	AgentCardURL string    `json:"agent_card_url,omitempty"`
	ProtocolVersion string  `json:"protocol_version,omitempty"`
	Skills       []A2ASkill `json:"skills,omitempty"`
	AuthToken    string    `json:"auth_token,omitempty"`
	Enabled      bool      `json:"enabled"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type A2ASkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ServiceToolState struct {
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type Skill struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Prompt      string    `json:"prompt"`
	Enabled     bool      `json:"enabled"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AgentPromptConfig struct {
	SystemPrompt            string    `json:"system_prompt"`
	CompressionSystemPrompt string    `json:"compression_system_prompt"`
	UpdatedAt               time.Time `json:"updated_at,omitempty"`
}

type AgentHabitState struct {
	LastSleepReviewDate     string    `json:"last_sleep_review_date,omitempty"`
	LastWakePlanDate        string    `json:"last_wake_plan_date,omitempty"`
	LastPromptEvolutionDate string    `json:"last_prompt_evolution_date,omitempty"`
	LastChatGreetingDate    string    `json:"last_chat_greeting_date,omitempty"`
	LastChatGreetingAt      string    `json:"last_chat_greeting_at,omitempty"`
	LastChatGreetingContent string    `json:"last_chat_greeting_content,omitempty"`
	UpdatedAt               time.Time `json:"updated_at,omitempty"`
}

type fileConfig struct {
	MCP struct {
		Services []Service `json:"services"`
	} `json:"mcp"`
	A2A struct {
		Agents []A2AAgent `json:"agents"`
	} `json:"a2a"`
	Skills struct {
		Items []Skill `json:"items"`
	} `json:"skills"`
	Agent struct {
		Prompts   AgentPromptConfig `json:"prompts"`
		Habits    AgentHabitState   `json:"habits"`
		Schedules []scheduler.Task  `json:"schedules"`
	} `json:"agent"`
}

type Store struct {
	path string
	mu   sync.RWMutex
	cfg  fileConfig
}

func DefaultAgentPromptConfig() AgentPromptConfig {
	return AgentPromptConfig{
		SystemPrompt:            strings.TrimSpace(agentprompt.DefaultSystemPrompt),
		CompressionSystemPrompt: strings.TrimSpace(agentprompt.DefaultCompressionSystemPrompt),
	}
}

func normalizeAgentPromptConfigOnLoad(cfg AgentPromptConfig) (AgentPromptConfig, bool) {
	normalized := cfg
	normalized.SystemPrompt = strings.TrimSpace(normalized.SystemPrompt)
	normalized.CompressionSystemPrompt = strings.TrimSpace(normalized.CompressionSystemPrompt)

	changed := normalized.SystemPrompt != cfg.SystemPrompt ||
		normalized.CompressionSystemPrompt != cfg.CompressionSystemPrompt
	return normalized, changed
}

func NewStore(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("settings file path is required")
	}

	s := &Store{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}
