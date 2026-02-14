package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"laughing-barnacle/internal/agentprompt"
)

type Config struct {
	Addr                       string
	SettingsFile               string
	ConversationFile           string
	LLMLogFile                 string
	CerberBaseURL              string
	CerberAPIKey               string
	CerberModel                string
	RequestTimeout             time.Duration
	MCPRequestTimeout          time.Duration
	MCPProtocolVersion         string
	MCPToolCacheTTL            time.Duration
	Temperature                float64
	MaxRecentMessages          int
	CompressionTriggerMessages int
	CompressionTriggerChars    int
	KeepRecentAfterCompression int
	MaxCompressionLoopsPerTurn int
	MaxToolCallRounds          int
	LLMLogLimit                int
	AgentSystemPrompt          string
	CompressionSystemPrompt    string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:                       envOrDefault("APP_ADDR", ":8080"),
		SettingsFile:               envOrDefault("APP_SETTINGS_FILE", "./data/settings.json"),
		ConversationFile:           envOrDefault("APP_CONVERSATION_FILE", "./data/conversation.json"),
		LLMLogFile:                 envOrDefault("APP_LLM_LOG_FILE", "./data/llm_logs.json"),
		CerberBaseURL:              envOrDefault("CERBER_BASE_URL", "https://api.cerber.ai"),
		CerberAPIKey:               os.Getenv("CERBER_API_KEY"),
		CerberModel:                envOrDefault("CERBER_MODEL", "gpt-4o-mini"),
		Temperature:                envFloat("CERBER_TEMPERATURE", 0.2),
		RequestTimeout:             envDuration("CERBER_TIMEOUT", 45*time.Second),
		MCPRequestTimeout:          envDuration("MCP_HTTP_TIMEOUT", 20*time.Second),
		MCPProtocolVersion:         envOrDefault("MCP_PROTOCOL_VERSION", "2025-06-18"),
		MCPToolCacheTTL:            envDuration("MCP_TOOL_CACHE_TTL", 30*time.Second),
		MaxRecentMessages:          envInt("AGENT_MAX_RECENT_MESSAGES", 14),
		CompressionTriggerMessages: envInt("AGENT_COMPRESSION_TRIGGER_MESSAGES", 20),
		CompressionTriggerChars:    envInt("AGENT_COMPRESSION_TRIGGER_CHARS", 14000),
		KeepRecentAfterCompression: envInt("AGENT_KEEP_RECENT_AFTER_COMPRESSION", 8),
		MaxCompressionLoopsPerTurn: envInt("AGENT_MAX_COMPRESSION_LOOPS", 3),
		MaxToolCallRounds:          envInt("AGENT_MAX_TOOL_CALL_ROUNDS", 6),
		LLMLogLimit:                envInt("APP_LLM_LOG_LIMIT", 500),
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
