package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"laughing-barnacle/internal/agentprompt"
)

type Config struct {
	Addr                        string
	SettingsFile                string
	SkillsDir                   string
	SkillsStateFile             string
	ConversationFile            string
	MemoryFile                  string
	LLMLogFile                  string
	CerberBaseURL               string
	CerberAPIKey                string
	CerberModel                 string
	CerberMaxRetries            int
	CerberRetryBaseDelay        time.Duration
	CerberRetryMaxDelay         time.Duration
	RequestTimeout              time.Duration
	MCPRequestTimeout           time.Duration
	MCPProtocolVersion          string
	MCPToolCacheTTL             time.Duration
	Temperature                 float64
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
	LLMLogLimit                 int
	AgentSystemPrompt           string
	CompressionSystemPrompt     string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:                        envOrDefault("APP_ADDR", ":8080"),
		SettingsFile:                envOrDefault("APP_SETTINGS_FILE", "./data/settings.json"),
		SkillsDir:                   envOrDefault("APP_SKILLS_DIR", "./data/skills"),
		SkillsStateFile:             envOrDefault("APP_SKILLS_STATE_FILE", "./data/skills_state.json"),
		ConversationFile:            envOrDefault("APP_CONVERSATION_FILE", "./data/conversation.json"),
		MemoryFile:                  envOrDefault("APP_MEMORY_FILE", "./data/memory.db"),
		LLMLogFile:                  envOrDefault("APP_LLM_LOG_FILE", "./data/llm_logs.json"),
		CerberBaseURL:               envOrDefault("CERBER_BASE_URL", "https://api.cerber.ai"),
		CerberAPIKey:                os.Getenv("CERBER_API_KEY"),
		CerberModel:                 envOrDefault("CERBER_MODEL", "gpt-4o-mini"),
		CerberMaxRetries:            envInt("CERBER_MAX_RETRIES", 2),
		CerberRetryBaseDelay:        envDuration("CERBER_RETRY_BASE_DELAY", 700*time.Millisecond),
		CerberRetryMaxDelay:         envDuration("CERBER_RETRY_MAX_DELAY", 8*time.Second),
		Temperature:                 envFloat("CERBER_TEMPERATURE", 0.2),
		RequestTimeout:              envDuration("CERBER_TIMEOUT", 45*time.Second),
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
		MemoryExtractionModel:       envOrDefault("AGENT_MEMORY_EXTRACTION_MODEL", envOrDefault("CERBER_MODEL", "gpt-4o-mini")),
		MemoryExtractionTemperature: envFloat("AGENT_MEMORY_EXTRACTION_TEMPERATURE", 0),
		LLMLogLimit:                 envInt("APP_LLM_LOG_LIMIT", 500),
		AgentSystemPrompt: envOrDefault("AGENT_SYSTEM_PROMPT",
			agentprompt.DefaultSystemPrompt),
		CompressionSystemPrompt: envOrDefault("AGENT_COMPRESSION_SYSTEM_PROMPT",
			agentprompt.DefaultCompressionSystemPrompt),
	}

	if cfg.CerberAPIKey == "" {
		return Config{}, fmt.Errorf("CERBER_API_KEY is required")
	}
	if cfg.MaxRecentMessages <= 0 {
		return Config{}, fmt.Errorf("AGENT_MAX_RECENT_MESSAGES must be > 0")
	}
	if cfg.CerberMaxRetries < 0 {
		return Config{}, fmt.Errorf("CERBER_MAX_RETRIES must be >= 0")
	}
	if cfg.CerberRetryBaseDelay <= 0 {
		return Config{}, fmt.Errorf("CERBER_RETRY_BASE_DELAY must be > 0")
	}
	if cfg.CerberRetryMaxDelay <= 0 {
		return Config{}, fmt.Errorf("CERBER_RETRY_MAX_DELAY must be > 0")
	}
	if cfg.CerberRetryMaxDelay < cfg.CerberRetryBaseDelay {
		return Config{}, fmt.Errorf("CERBER_RETRY_MAX_DELAY must be >= CERBER_RETRY_BASE_DELAY")
	}
	if cfg.KeepRecentAfterCompression < 0 {
		return Config{}, fmt.Errorf("AGENT_KEEP_RECENT_AFTER_COMPRESSION must be >= 0")
	}
	if cfg.MaxCompressionLoopsPerTurn <= 0 {
		return Config{}, fmt.Errorf("AGENT_MAX_COMPRESSION_LOOPS must be > 0")
	}
	if cfg.MaxToolCallRounds <= 0 {
		return Config{}, fmt.Errorf("AGENT_MAX_TOOL_CALL_ROUNDS must be > 0")
	}
	if cfg.LLMLogLimit <= 0 {
		return Config{}, fmt.Errorf("APP_LLM_LOG_LIMIT must be > 0")
	}
	if cfg.LLMLogFile == "" {
		return Config{}, fmt.Errorf("APP_LLM_LOG_FILE is required")
	}
	if cfg.ConversationFile == "" {
		return Config{}, fmt.Errorf("APP_CONVERSATION_FILE is required")
	}
	if cfg.MemoryFile == "" {
		return Config{}, fmt.Errorf("APP_MEMORY_FILE is required")
	}
	if cfg.MemoryIdleWindow <= 0 {
		return Config{}, fmt.Errorf("AGENT_MEMORY_IDLE_WINDOW must be > 0")
	}
	if cfg.MemoryMaxSegmentWindow <= 0 {
		return Config{}, fmt.Errorf("AGENT_MEMORY_MAX_SEGMENT_WINDOW must be > 0")
	}
	if cfg.MemoryMaxSegmentMessages <= 0 {
		return Config{}, fmt.Errorf("AGENT_MEMORY_MAX_SEGMENT_MESSAGES must be > 0")
	}
	if cfg.MemoryWorkerInterval <= 0 {
		return Config{}, fmt.Errorf("AGENT_MEMORY_WORKER_INTERVAL must be > 0")
	}
	if cfg.MemoryTrashTTL <= 0 {
		return Config{}, fmt.Errorf("AGENT_MEMORY_TRASH_TTL must be > 0")
	}
	if cfg.MemoryFailedRetryAfter <= 0 {
		return Config{}, fmt.Errorf("AGENT_MEMORY_FAILED_RETRY_AFTER must be > 0")
	}
	if cfg.MemoryExtractionUseLLM && strings.TrimSpace(cfg.MemoryExtractionModel) == "" {
		return Config{}, fmt.Errorf("AGENT_MEMORY_EXTRACTION_MODEL is required when AGENT_MEMORY_EXTRACTION_USE_LLM=true")
	}
	if cfg.SkillsDir == "" {
		return Config{}, fmt.Errorf("APP_SKILLS_DIR is required")
	}
	if cfg.SkillsStateFile == "" {
		return Config{}, fmt.Errorf("APP_SKILLS_STATE_FILE is required")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
