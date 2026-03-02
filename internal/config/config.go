package config

import (
	"fmt"
	"strings"
	"time"

	"laughing-barnacle/internal/agentprompt"
	"laughing-barnacle/internal/localapi"
)

type Config struct {
	Addr             string
	LocalAPIBaseURL  string
	SettingsFile     string
	SkillsDir        string
	BuiltinSkillsDir string
	SkillsStateFile  string
	ConversationFile string
	MemoryFile       string
	LLMLogFile       string

	AgentModel      string
	Temperature     float64
	RequestTimeout  time.Duration
	LLMLogLimit     int
	StartupWarnings []string

	LLMGatewayDefaultProvider string
	LLMGatewayDefaultModel    string

	LLMGatewayCerberBaseURL        string
	LLMGatewayCerberAPIKey         string
	LLMGatewayCerberMaxRetries     int
	LLMGatewayCerberRetryBaseDelay time.Duration
	LLMGatewayCerberRetryMaxDelay  time.Duration

	LLMGatewayOpenAICodexBaseURL         string
	LLMGatewayOpenAICodexAPIToken        string
	LLMGatewayOpenAICodexAuthFilePath    string
	LLMGatewayOpenAICodexTransport       string
	LLMGatewayOpenAICodexReasoningEffort string
	LLMGatewayOpenAICodexMaxRetries      int

	MCPRequestTimeout           time.Duration
	MCPProtocolVersion          string
	MCPToolCacheTTL             time.Duration
	MaxRecentMessages           int
	CompressionTriggerMessages  int
	CompressionTriggerChars     int
	KeepRecentAfterCompression  int
	MaxCompressionLoopsPerTurn  int
	MaxToolCallRounds           int
	MemoryIdleWindow            time.Duration
	MemoryMaxSegmentWindow      time.Duration
	MemoryMaxSegmentMessages    int
	MemoryWorkerInterval        time.Duration
	MemoryTrashTTL              time.Duration
	MemoryFailedRetryAfter      time.Duration
	MemoryExtractionUseLLM      bool
	MemoryExtractionFallback    bool
	MemoryExtractionModel       string
	MemoryExtractionTemperature float64

	AgentSystemPrompt       string
	CompressionSystemPrompt string
}

func Load() (Config, error) {
	llmEnv := loadLLMGatewayEnv()
	cfg := newConfig(llmEnv)
	cfg.LocalAPIBaseURL = localapi.ResolveBaseURL(cfg.Addr)
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func newConfig(llmEnv llmGatewayEnv) Config {
	defaultModelRef := llmEnv.DefaultProvider + "/" + llmEnv.DefaultModel
	return Config{
		Addr:             envOrDefault("APP_ADDR", ":8080"),
		SettingsFile:     envOrDefault("APP_SETTINGS_FILE", "./data/settings.json"),
		SkillsDir:        envOrDefault("APP_SKILLS_DIR", "./data/skills"),
		BuiltinSkillsDir: envOrDefault("APP_BUILTIN_SKILLS_DIR", "./builtin-skills"),
		SkillsStateFile:  envOrDefault("APP_SKILLS_STATE_FILE", "./data/skills_state.json"),
		ConversationFile: envOrDefault("APP_CONVERSATION_FILE", "./data/conversation.json"),
		MemoryFile:       envOrDefault("APP_MEMORY_FILE", "./data/memory.db"),
		LLMLogFile:       envOrDefault("APP_LLM_LOG_FILE", "./data/llm_logs.json"),

		AgentModel:      envOrDefault("AGENT_LLM_MODEL", defaultModelRef),
		Temperature:     envFloat("CERBER_TEMPERATURE", 0.2),
		RequestTimeout:  llmEnv.RequestTimeout,
		LLMLogLimit:     envInt("APP_LLM_LOG_LIMIT", 500),
		StartupWarnings: append([]string(nil), llmEnv.Warnings...),

		LLMGatewayDefaultProvider: llmEnv.DefaultProvider,
		LLMGatewayDefaultModel:    llmEnv.DefaultModel,

		LLMGatewayCerberBaseURL:        llmEnv.CerberBaseURL,
		LLMGatewayCerberAPIKey:         llmEnv.CerberAPIKey,
		LLMGatewayCerberMaxRetries:     llmEnv.CerberMaxRetries,
		LLMGatewayCerberRetryBaseDelay: llmEnv.CerberRetryBaseDelay,
		LLMGatewayCerberRetryMaxDelay:  llmEnv.CerberRetryMaxDelay,

		LLMGatewayOpenAICodexBaseURL:         llmEnv.OpenAICodexBaseURL,
		LLMGatewayOpenAICodexAPIToken:        llmEnv.OpenAICodexAPIToken,
		LLMGatewayOpenAICodexAuthFilePath:    llmEnv.OpenAICodexAuthFilePath,
		LLMGatewayOpenAICodexTransport:       llmEnv.OpenAICodexTransport,
		LLMGatewayOpenAICodexReasoningEffort: llmEnv.OpenAICodexReasoningEffort,
		LLMGatewayOpenAICodexMaxRetries:      llmEnv.OpenAICodexMaxRetries,

		MCPRequestTimeout:           envDuration("MCP_HTTP_TIMEOUT", 20*time.Second),
		MCPProtocolVersion:          envOrDefault("MCP_PROTOCOL_VERSION", "2025-06-18"),
		MCPToolCacheTTL:             envDuration("MCP_TOOL_CACHE_TTL", 30*time.Second),
		MaxRecentMessages:           envInt("AGENT_MAX_RECENT_MESSAGES", 10),
		CompressionTriggerMessages:  envInt("AGENT_COMPRESSION_TRIGGER_MESSAGES", 20),
		CompressionTriggerChars:     envInt("AGENT_COMPRESSION_TRIGGER_CHARS", 9000),
		KeepRecentAfterCompression:  envInt("AGENT_KEEP_RECENT_AFTER_COMPRESSION", 8),
		MaxCompressionLoopsPerTurn:  envInt("AGENT_MAX_COMPRESSION_LOOPS", 3),
		MaxToolCallRounds:           envInt("AGENT_MAX_TOOL_CALL_ROUNDS", 10),
		MemoryIdleWindow:            envDuration("AGENT_MEMORY_IDLE_WINDOW", 5*time.Minute),
		MemoryMaxSegmentWindow:      envDuration("AGENT_MEMORY_MAX_SEGMENT_WINDOW", 10*time.Minute),
		MemoryMaxSegmentMessages:    envInt("AGENT_MEMORY_MAX_SEGMENT_MESSAGES", 8),
		MemoryWorkerInterval:        envDuration("AGENT_MEMORY_WORKER_INTERVAL", 30*time.Second),
		MemoryTrashTTL:              envDuration("AGENT_MEMORY_TRASH_TTL", 30*24*time.Hour),
		MemoryFailedRetryAfter:      envDuration("AGENT_MEMORY_FAILED_RETRY_AFTER", 2*time.Minute),
		MemoryExtractionUseLLM:      envBool("AGENT_MEMORY_EXTRACTION_USE_LLM", true),
		MemoryExtractionFallback:    envBool("AGENT_MEMORY_EXTRACTION_FALLBACK", true),
		MemoryExtractionModel:       envOrDefault("AGENT_MEMORY_EXTRACTION_MODEL", defaultModelRef),
		MemoryExtractionTemperature: envFloat("AGENT_MEMORY_EXTRACTION_TEMPERATURE", 0),

		AgentSystemPrompt:       envOrDefault("AGENT_SYSTEM_PROMPT", agentprompt.DefaultSystemPrompt),
		CompressionSystemPrompt: envOrDefault("AGENT_COMPRESSION_SYSTEM_PROMPT", agentprompt.DefaultCompressionSystemPrompt),
	}
}

func (c Config) validate() error {
	if err := c.validateGateway(); err != nil {
		return err
	}
	if err := c.validateAgentRuntime(); err != nil {
		return err
	}
	return c.validateStorage()
}

func (c Config) validateGateway() error {
	switch c.LLMGatewayDefaultProvider {
	case "cerber", "openai-codex":
	default:
		return fmt.Errorf("LLM_GATEWAY_DEFAULT_PROVIDER must be one of: cerber, openai-codex")
	}
	if c.LLMGatewayDefaultProvider == "cerber" && strings.TrimSpace(c.LLMGatewayCerberAPIKey) == "" {
		return fmt.Errorf("LLM_GATEWAY_CERBER_API_KEY is required when default provider is cerber")
	}
	if c.LLMGatewayCerberMaxRetries < 0 {
		return fmt.Errorf("LLM_GATEWAY_CERBER_MAX_RETRIES must be >= 0")
	}
	if c.LLMGatewayOpenAICodexMaxRetries < 0 {
		return fmt.Errorf("LLM_GATEWAY_OPENAI_CODEX_MAX_RETRIES must be >= 0")
	}
	switch strings.ToLower(strings.TrimSpace(c.LLMGatewayOpenAICodexReasoningEffort)) {
	case "", "minimal", "low", "medium", "high":
	default:
		return fmt.Errorf("LLM_GATEWAY_OPENAI_CODEX_REASONING_EFFORT must be one of: minimal, low, medium, high")
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("LLM_GATEWAY_REQUEST_TIMEOUT must be > 0")
	}
	if c.LLMGatewayCerberRetryBaseDelay <= 0 {
		return fmt.Errorf("LLM_GATEWAY_CERBER_RETRY_BASE_DELAY must be > 0")
	}
	if c.LLMGatewayCerberRetryMaxDelay <= 0 {
		return fmt.Errorf("LLM_GATEWAY_CERBER_RETRY_MAX_DELAY must be > 0")
	}
	if c.LLMGatewayCerberRetryMaxDelay < c.LLMGatewayCerberRetryBaseDelay {
		return fmt.Errorf("LLM_GATEWAY_CERBER_RETRY_MAX_DELAY must be >= LLM_GATEWAY_CERBER_RETRY_BASE_DELAY")
	}
	return nil
}

func (c Config) validateAgentRuntime() error {
	if c.MaxRecentMessages <= 0 {
		return fmt.Errorf("AGENT_MAX_RECENT_MESSAGES must be > 0")
	}
	if c.KeepRecentAfterCompression < 0 {
		return fmt.Errorf("AGENT_KEEP_RECENT_AFTER_COMPRESSION must be >= 0")
	}
	if c.MaxCompressionLoopsPerTurn <= 0 {
		return fmt.Errorf("AGENT_MAX_COMPRESSION_LOOPS must be > 0")
	}
	if c.MaxToolCallRounds <= 0 {
		return fmt.Errorf("AGENT_MAX_TOOL_CALL_ROUNDS must be > 0")
	}
	if c.MemoryIdleWindow <= 0 {
		return fmt.Errorf("AGENT_MEMORY_IDLE_WINDOW must be > 0")
	}
	if c.MemoryMaxSegmentWindow <= 0 {
		return fmt.Errorf("AGENT_MEMORY_MAX_SEGMENT_WINDOW must be > 0")
	}
	if c.MemoryMaxSegmentMessages <= 0 {
		return fmt.Errorf("AGENT_MEMORY_MAX_SEGMENT_MESSAGES must be > 0")
	}
	if c.MemoryWorkerInterval <= 0 {
		return fmt.Errorf("AGENT_MEMORY_WORKER_INTERVAL must be > 0")
	}
	if c.MemoryTrashTTL <= 0 {
		return fmt.Errorf("AGENT_MEMORY_TRASH_TTL must be > 0")
	}
	if c.MemoryFailedRetryAfter <= 0 {
		return fmt.Errorf("AGENT_MEMORY_FAILED_RETRY_AFTER must be > 0")
	}
	if c.MemoryExtractionUseLLM && strings.TrimSpace(c.MemoryExtractionModel) == "" {
		return fmt.Errorf("AGENT_MEMORY_EXTRACTION_MODEL is required when AGENT_MEMORY_EXTRACTION_USE_LLM=true")
	}
	return nil
}

func (c Config) validateStorage() error {
	if c.LLMLogLimit <= 0 {
		return fmt.Errorf("APP_LLM_LOG_LIMIT must be > 0")
	}
	if c.LLMLogFile == "" {
		return fmt.Errorf("APP_LLM_LOG_FILE is required")
	}
	if c.ConversationFile == "" {
		return fmt.Errorf("APP_CONVERSATION_FILE is required")
	}
	if c.MemoryFile == "" {
		return fmt.Errorf("APP_MEMORY_FILE is required")
	}
	if c.SkillsDir == "" {
		return fmt.Errorf("APP_SKILLS_DIR is required")
	}
	if c.BuiltinSkillsDir == "" {
		return fmt.Errorf("APP_BUILTIN_SKILLS_DIR is required")
	}
	if c.SkillsStateFile == "" {
		return fmt.Errorf("APP_SKILLS_STATE_FILE is required")
	}
	return nil
}
