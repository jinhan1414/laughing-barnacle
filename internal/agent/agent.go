package agent

import (
	"context"
	"regexp"
	"sync"
	"time"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

type Config struct {
	Model                      string
	Temperature                float64
	MaxRecentMessages          int
	CompressionTriggerMessages int
	CompressionTriggerChars    int
	KeepRecentAfterCompression int
	MaxCompressionLoopsPerTurn int
	MaxToolCallRounds          int
	SystemPrompt               string
	CompressionSystemPrompt    string
	EnforceHumanRoutine        bool
}

type ToolProvider interface {
	ListTools(ctx context.Context) ([]llm.ToolDefinition, error)
	CallTool(ctx context.Context, call llm.ToolCall) (string, error)
}

type SkillProvider interface {
	ListEnabledSkillIndex() []string
	ReadEnabledSkillPrompt(skillID string) (string, bool)
}

type MemoryProvider interface {
	ListIndexLines(limit int) []string
	AppendTurn(user, assistant string, toolCalls []conversation.ToolCall, now time.Time) error
}

type AutoSkillWriter interface {
	UpsertAutoSkill(name, prompt string) error
}

type evolvedSkill struct {
	Name   string
	Prompt string
}

const (
	maxInjectedSkillPrompts    = 4
	maxSingleSkillPromptRunes  = 220
	minInjectedSkillScore      = 3
	maxSkillFocusUserMessages  = 3
	maxNightEvolvedSkills      = 3
	maxEvolvedSkillNameRunes   = 24
	maxEvolvedSkillPromptRunes = 180
	maxScheduledRecentMessages = 20
	builtinLinuxBashToolName   = "linux__bash"
	defaultBashTimeoutSeconds  = 20
	maxBashTimeoutSeconds      = 180
	maxBashStdoutRunes         = 4000
	maxBashStderrRunes         = 2000
	maxGreetingRecentMessages  = 8
	maxGreetingTaskStatuses    = 5
	maxSummaryForRequestRunes  = 1400
	maxContextMessageRunes     = 900
	maxRecentContextRunes      = 4200
	maxReplayHistoryToolCalls  = 2
	maxAssistantReplyRunes     = 2200
	runtimeDateContextMarker   = "[[RUNTIME_DATE_CONTEXT]]"
)

var (
	skillTokenPattern         = regexp.MustCompile(`[\p{Han}]{2,8}|[a-zA-Z][a-zA-Z0-9_-]{2,}`)
	comparableStripPattern    = regexp.MustCompile(`[[:space:][:punct:]，。！？；：、“”‘’（）【】《》·]+`)
	numberedListPrefixPattern = regexp.MustCompile(`^\d+[.)、]\s*`)
	runLinuxBashFn            = runLinuxBash
)

type PromptProvider interface {
	GetSystemPrompt() string
	GetCompressionSystemPrompt() string
}

type PromptUpdater interface {
	UpdateAgentPrompts(systemPrompt, compressionSystemPrompt string) error
}

type HabitProvider interface {
	GetLastSleepReviewDate() string
	GetLastWakePlanDate() string
	GetLastPromptEvolutionDate() string
	SetLastSleepReviewDate(date string) error
	SetLastWakePlanDate(date string) error
	SetLastPromptEvolutionDate(date string) error
}

type ChatGreetingInput struct {
	Now                 time.Time
	IsFirstToday        bool
	LastGreetingAt      time.Time
	LastGreetingContent string
	RecentTaskStatuses  []string
}

type Agent struct {
	cfg     Config
	llm     llm.Client
	tools   ToolProvider
	skills  SkillProvider
	memory  MemoryProvider
	prompts PromptProvider
	updater PromptUpdater
	habits  HabitProvider
	store   *conversation.Store
	nowFn   func() time.Time
	mu      sync.Mutex
}

func New(cfg Config, store *conversation.Store, llmClient llm.Client, tools ToolProvider) *Agent {
	return &Agent{
		cfg:   cfg,
		llm:   llmClient,
		tools: tools,
		store: store,
		nowFn: time.Now,
	}
}

func (a *Agent) SetSkillProvider(provider SkillProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.skills = provider
}

func (a *Agent) SetMemoryProvider(provider MemoryProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.memory = provider
}

func (a *Agent) SetPromptProvider(provider PromptProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prompts = provider
}

func (a *Agent) SetPromptUpdater(updater PromptUpdater) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.updater = updater
}

func (a *Agent) SetHabitProvider(provider HabitProvider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.habits = provider
}

func (a *Agent) GetEffectivePrompts() (systemPrompt string, compressionSystemPrompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.resolvePromptsLocked()
}
