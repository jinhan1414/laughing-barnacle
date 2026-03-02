package config

import (
	"strings"
	"testing"
)

func TestLoad_UsesBuiltInPersonaPromptByDefault(t *testing.T) {
	t.Setenv("CERBER_API_KEY", "test-key")
	t.Setenv("AGENT_SYSTEM_PROMPT", "")
	t.Setenv("AGENT_COMPRESSION_SYSTEM_PROMPT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if !strings.Contains(cfg.AgentSystemPrompt, "傻毛") {
		t.Fatalf("expected built-in persona prompt, got %q", cfg.AgentSystemPrompt)
	}
	if !strings.Contains(cfg.AgentSystemPrompt, "不使用表情符号") {
		t.Fatalf("expected no-emoji instruction in built-in prompt")
	}
	if !strings.Contains(cfg.CompressionSystemPrompt, "上下文压缩器") {
		t.Fatalf("unexpected built-in compression prompt: %q", cfg.CompressionSystemPrompt)
	}
}

func TestLoad_PromptEnvOverrideWorks(t *testing.T) {
	t.Setenv("CERBER_API_KEY", "test-key")
	t.Setenv("AGENT_SYSTEM_PROMPT", "custom-system")
	t.Setenv("AGENT_COMPRESSION_SYSTEM_PROMPT", "custom-compress")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.AgentSystemPrompt != "custom-system" {
		t.Fatalf("expected custom system prompt, got %q", cfg.AgentSystemPrompt)
	}
	if cfg.CompressionSystemPrompt != "custom-compress" {
		t.Fatalf("expected custom compression prompt, got %q", cfg.CompressionSystemPrompt)
	}
}

func TestLoad_DefaultMaxToolCallRoundsIsTen(t *testing.T) {
	t.Setenv("CERBER_API_KEY", "test-key")
	t.Setenv("AGENT_MAX_TOOL_CALL_ROUNDS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.MaxToolCallRounds != 10 {
		t.Fatalf("expected default MaxToolCallRounds=10, got %d", cfg.MaxToolCallRounds)
	}
}

func TestLoad_LocalAPIBaseURLFollowsAPPAddr(t *testing.T) {
	t.Setenv("CERBER_API_KEY", "test-key")
	t.Setenv("APP_ADDR", ":9080")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.LocalAPIBaseURL != "http://127.0.0.1:9080" {
		t.Fatalf("expected LocalAPIBaseURL to follow APP_ADDR, got %q", cfg.LocalAPIBaseURL)
	}
}

func TestLoad_DefaultBuiltinSkillsDir(t *testing.T) {
	t.Setenv("CERBER_API_KEY", "test-key")
	t.Setenv("APP_BUILTIN_SKILLS_DIR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.BuiltinSkillsDir != "./builtin-skills" {
		t.Fatalf("expected default BuiltinSkillsDir, got %q", cfg.BuiltinSkillsDir)
	}
}

func TestLoad_LegacyCerberEnvMappedWithWarning(t *testing.T) {
	t.Setenv("CERBER_API_KEY", "legacy-key")
	t.Setenv("LLM_GATEWAY_CERBER_API_KEY", "")
	t.Setenv("LLM_GATEWAY_DEFAULT_PROVIDER", "cerber")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.LLMGatewayCerberAPIKey != "legacy-key" {
		t.Fatalf("expected mapped legacy API key, got %q", cfg.LLMGatewayCerberAPIKey)
	}
	if len(cfg.StartupWarnings) == 0 {
		t.Fatalf("expected legacy mapping warning")
	}
}

func TestLoad_UsesGatewayDefaultModelRefByDefault(t *testing.T) {
	t.Setenv("CERBER_API_KEY", "test-key")
	t.Setenv("AGENT_LLM_MODEL", "")
	t.Setenv("LLM_GATEWAY_DEFAULT_PROVIDER", "openai-codex")
	t.Setenv("LLM_GATEWAY_DEFAULT_MODEL", "gpt-5-codex")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.AgentModel != "openai-codex/gpt-5-codex" {
		t.Fatalf("expected provider/model default ref, got %q", cfg.AgentModel)
	}
}

func TestLoad_DefaultOpenAICodexReasoningEffortIsHigh(t *testing.T) {
	t.Setenv("CERBER_API_KEY", "test-key")
	t.Setenv("LLM_GATEWAY_OPENAI_CODEX_REASONING_EFFORT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.LLMGatewayOpenAICodexReasoningEffort != "high" {
		t.Fatalf("expected default reasoning effort high, got %q", cfg.LLMGatewayOpenAICodexReasoningEffort)
	}
}
