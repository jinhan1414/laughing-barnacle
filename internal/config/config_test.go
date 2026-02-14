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
