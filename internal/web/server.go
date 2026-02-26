package web

import (
	"embed"
	"html/template"
	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llmlog"
	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/memory"
	"laughing-barnacle/internal/skills"
	"time"
)

//go:embed templates/*.html
var embeddedTemplates embed.FS

type Server struct {
	agent       *agent.Agent
	convStore   *conversation.Store
	logStore    *llmlog.Store
	mcpStore    *mcp.Store
	mcpTools    *mcp.ToolProvider
	skillStore  *skills.Store
	memoryStore *memory.Store
	scheduler   ScheduleReloader
	memoryWorkerConfig
	tmpl *template.Template
}

type memoryWorkerConfig struct {
	Interval    time.Duration
	IdleWindow  time.Duration
	MaxWindow   time.Duration
	MaxMessages int
}

type ScheduleReloader interface {
	Reload() error
	RunNow(taskID string) error
}

type scheduleRunningChecker interface {
	HasRunningTask() bool
}

type chatPageData struct {
	Timeline       []chatTimelineItem
	Error          string
	RetryAvailable bool
	Draft          string
}

type chatTimelineItem struct {
	Kind           string
	Content        string
	ToolCalls      []conversation.ToolCall
	Usage          *conversation.TokenUsage
	CreatedAt      time.Time
	ShowTimestamp  bool
	TimestampLabel string
}

type logsPageData struct {
	Entries []llmlog.Entry
}

type settingsSection struct {
	Key         string
	Title       string
	Description string
}

type mcpServiceView struct {
	ID          string
	Name        string
	Endpoint    string
	Command     string
	Args        string
	Transport   string
	Enabled     bool
	UpdatedAt   string
	Connected   bool
	ToolCount   int
	Tools       []mcpServiceToolView
	StatusLabel string
	StatusError string
}

type mcpServiceToolView struct {
	Name        string
	Description string
	Enabled     bool
}

type settingsPageData struct {
	ActiveSection  string
	Sections       []settingsSection
	Services       []mcpServiceView
	A2AAgents      []a2aAgentView
	Skills         []skillView
	MemoryTask     scheduleMemoryMaintenanceView
	MemoryMetrics  memoryMetricsView
	MemoryNodes    []memoryNodeView
	MemoryPending  []memoryPendingView
	MemorySegments []memorySegmentView
	Schedules      []scheduledTaskView
	AgentPrompts   agentPromptsView
	Success        string
	Error          string
}

type a2aAgentView struct {
	ID           string
	Name         string
	Description  string
	Endpoint     string
	AgentCardURL string
	HasAuthToken bool
	Enabled      bool
	UpdatedAt    string
}

type skillView struct {
	ID          string
	Name        string
	Description string
	Prompt      string
	Source      string
	Enabled     bool
	UpdatedAt   string
}

type memoryNodeView struct {
	Path       string
	Title      string
	Type       string
	SchemaKind string
	Summary    string
	Revision   int64
	UpdatedAt  string
}

type memorySegmentView struct {
	ID             string
	Status         string
	RetryCount     int
	Turns          int
	CloseReason    string
	LastUserAt     string
	UpdatedAt      string
	PersistedPaths []string
	Error          string
}

type memoryPendingView struct {
	Path       string
	TargetPath string
	Title      string
	Summary    string
	Confidence string
	UpdatedAt  string
}

type memoryMetricsView struct {
	SegmentTotal      int
	SegmentOpen       int
	SegmentClosed     int
	SegmentProcessing int
	SegmentPersisted  int
	SegmentFailed     int
	FailedRate        string
	RetryTotal        int
	PendingCount      int
	ReviewedCount     int
	LastPersistedAt   string
	WarningFailRate   bool
	WarningPending    bool
	WarningRetry      bool
}

type scheduleMemoryMaintenanceView struct {
	Available        bool
	Driver           string
	Interval         string
	IdleWindow       string
	MaxWindow        string
	MaxMessages      int
	SegmentTotal     int
	SegmentPersisted int
	SegmentFailed    int
	FailedRate       string
	RetryTotal       int
	PendingCount     int
	LastRunAt        string
	LastRunDetail    string
	LastPersistedAt  string
	WarningFailRate  bool
	WarningPending   bool
	WarningRetry     bool
}

type agentPromptsView struct {
	SystemPrompt            string
	CompressionSystemPrompt string
	UpdatedAt               string
}

type scheduledTaskView struct {
	ID          string
	Name        string
	Description string
	Action      string
	ActionLabel string
	CronExpr    string
	Enabled     bool
	UpdatedAt   string
	LastRunAt   string
	LastStatus  string
	LastMessage string
}

type apiMCPService struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Transport string    `json:"transport"`
	Endpoint  string    `json:"endpoint,omitempty"`
	Command   string    `json:"command,omitempty"`
	Args      []string  `json:"args,omitempty"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type apiA2AAgent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Endpoint     string    `json:"endpoint"`
	AgentCardURL string    `json:"agent_card_url,omitempty"`
	Enabled      bool      `json:"enabled"`
	HasAuthToken bool      `json:"has_auth_token"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

type apiSkill struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Source      string    `json:"source,omitempty"`
	Enabled     bool      `json:"enabled"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type apiScheduledTask struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Action      string    `json:"action"`
	CronExpr    string    `json:"cron_expr"`
	Enabled     bool      `json:"enabled"`
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
	LastStatus  string    `json:"last_status,omitempty"`
	LastMessage string    `json:"last_message,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type apiMemoryIndexItem struct {
	Path      string          `json:"path"`
	Title     string          `json:"title"`
	Type      memory.NodeType `json:"type"`
	Summary   string          `json:"summary,omitempty"`
	Revision  int64           `json:"revision"`
	UpdatedAt time.Time       `json:"updated_at,omitempty"`
}

type apiChatUpdate struct {
	Kind        string         `json:"kind"`
	Content     string         `json:"content"`
	CreatedAtUS int64          `json:"created_at_us"`
	Usage       *apiTokenUsage `json:"usage,omitempty"`
}

type apiTokenUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type apiChatToolRunRequest struct {
	Tool string `json:"tool"`
}

type apiChatToolRunResponse struct {
	OK         bool   `json:"ok"`
	Tool       string `json:"tool"`
	Compressed bool   `json:"compressed"`
	Message    string `json:"message"`
}

type chatGreetResponse struct {
	Created bool   `json:"created"`
	Content string `json:"content,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

const (
	chatGreetingCooldown           = 30 * time.Minute
	chatTimestampGap               = 5 * time.Minute
	defaultMemoryWorkerInterval    = 30 * time.Second
	defaultMemoryIdleWindow        = 5 * time.Minute
	defaultMemoryMaxSegmentWindow  = 10 * time.Minute
	defaultMemoryMaxSegmentMessage = 8
)
