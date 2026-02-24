package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreUpsertAgentPromptConfig_Persisted(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertAgentPromptConfig(AgentPromptConfig{
		SystemPrompt:            "你是数字分身，保持一致人格。",
		CompressionSystemPrompt: "你负责压缩对话，保留事实和待办。",
	}); err != nil {
		t.Fatalf("UpsertAgentPromptConfig error: %v", err)
	}

	cfg := store.GetAgentPromptConfig()
	if cfg.SystemPrompt != "你是数字分身，保持一致人格。" {
		t.Fatalf("unexpected system prompt: %q", cfg.SystemPrompt)
	}
	if cfg.CompressionSystemPrompt != "你负责压缩对话，保留事实和待办。" {
		t.Fatalf("unexpected compression prompt: %q", cfg.CompressionSystemPrompt)
	}
	if cfg.UpdatedAt.IsZero() {
		t.Fatalf("expected updated_at to be set")
	}

	reloaded, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}
	reloadedCfg := reloaded.GetAgentPromptConfig()
	if reloadedCfg.SystemPrompt != cfg.SystemPrompt {
		t.Fatalf("unexpected system prompt after reload: %q", reloadedCfg.SystemPrompt)
	}
	if reloadedCfg.CompressionSystemPrompt != cfg.CompressionSystemPrompt {
		t.Fatalf("unexpected compression prompt after reload: %q", reloadedCfg.CompressionSystemPrompt)
	}
}

func TestStoreUpsertAgentPromptConfig_RequiresBothWhenConfigured(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertAgentPromptConfig(AgentPromptConfig{
		SystemPrompt: "only system",
	}); err == nil {
		t.Fatalf("expected error when compression prompt is missing")
	}
	if err := store.UpsertAgentPromptConfig(AgentPromptConfig{
		CompressionSystemPrompt: "only compression",
	}); err == nil {
		t.Fatalf("expected error when system prompt is missing")
	}
}

func TestStoreInitializesDefaultAgentPromptConfig(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	cfg := store.GetAgentPromptConfig()
	if !strings.Contains(cfg.SystemPrompt, "傻毛") {
		t.Fatalf("expected default persona prompt, got %q", cfg.SystemPrompt)
	}
	if !strings.Contains(cfg.SystemPrompt, "不使用表情符号") {
		t.Fatalf("expected no-emoji preference in default prompt")
	}
	if strings.TrimSpace(cfg.CompressionSystemPrompt) == "" {
		t.Fatalf("expected default compression prompt")
	}
}

func TestStoreLoad_LegacyDeprecatedPrompt_AutoResetToDefault(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	raw := `{
  "agent": {
    "prompts": {
      "system_prompt": "你是用户的 AI 数字分身，名字叫“傻毛”，女性，8 年全栈开发经验。\n数字分身长期目标：持续记录并改进生活、工作、学习。\n持续提升机制：每次交互尽量给出 1-3 条可执行改进建议。\n不使用表情符号。",
      "compression_system_prompt": "legacy-compression"
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("write settings file error: %v", err)
	}

	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	cfg := store.GetAgentPromptConfig()
	defaultCfg := DefaultAgentPromptConfig()
	if cfg.SystemPrompt != defaultCfg.SystemPrompt {
		t.Fatalf("expected legacy deprecated system prompt to reset to default")
	}
	if cfg.CompressionSystemPrompt != defaultCfg.CompressionSystemPrompt {
		t.Fatalf("expected compression prompt reset to default with legacy deprecated prompt")
	}

	reloaded, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}
	reloadedCfg := reloaded.GetAgentPromptConfig()
	if reloadedCfg.SystemPrompt != defaultCfg.SystemPrompt {
		t.Fatalf("expected reset prompt persisted after reload")
	}
}

func TestStoreResetAgentPromptConfig(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertAgentPromptConfig(AgentPromptConfig{
		SystemPrompt:            "custom-system",
		CompressionSystemPrompt: "custom-compression",
	}); err != nil {
		t.Fatalf("UpsertAgentPromptConfig error: %v", err)
	}

	if err := store.ResetAgentPromptConfig(); err != nil {
		t.Fatalf("ResetAgentPromptConfig error: %v", err)
	}

	cfg := store.GetAgentPromptConfig()
	if !strings.Contains(cfg.SystemPrompt, "傻毛") {
		t.Fatalf("expected reset to default prompt, got %q", cfg.SystemPrompt)
	}
	if !strings.Contains(cfg.CompressionSystemPrompt, "上下文压缩器") {
		t.Fatalf("expected reset to default compression prompt, got %q", cfg.CompressionSystemPrompt)
	}
	if cfg.UpdatedAt.IsZero() {
		t.Fatalf("expected updated time on reset")
	}
}

func TestStoreAgentHabitState_Persisted(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.SetLastSleepReviewDate("2026-02-14"); err != nil {
		t.Fatalf("SetLastSleepReviewDate error: %v", err)
	}
	if err := store.SetLastWakePlanDate("2026-02-14"); err != nil {
		t.Fatalf("SetLastWakePlanDate error: %v", err)
	}
	if err := store.SetLastPromptEvolutionDate("2026-02-14"); err != nil {
		t.Fatalf("SetLastPromptEvolutionDate error: %v", err)
	}
	greetAt := time.Date(2026, 2, 14, 8, 30, 0, 0, time.FixedZone("CST", 8*3600))
	if err := store.SetLastChatGreetingState("2026-02-14", greetAt, "早上好，欢迎回来。"); err != nil {
		t.Fatalf("SetLastChatGreetingState error: %v", err)
	}

	if got := store.GetLastSleepReviewDate(); got != "2026-02-14" {
		t.Fatalf("unexpected sleep review date: %q", got)
	}
	if got := store.GetLastWakePlanDate(); got != "2026-02-14" {
		t.Fatalf("unexpected wake plan date: %q", got)
	}
	if got := store.GetLastPromptEvolutionDate(); got != "2026-02-14" {
		t.Fatalf("unexpected prompt evolution date: %q", got)
	}
	if got := store.GetLastChatGreetingDate(); got != "2026-02-14" {
		t.Fatalf("unexpected chat greeting date: %q", got)
	}
	gotGreetAt := store.GetLastChatGreetingAt()
	if gotGreetAt.IsZero() || !gotGreetAt.Equal(greetAt.UTC()) {
		t.Fatalf("unexpected chat greeting time: %v", gotGreetAt)
	}
	if got := store.GetLastChatGreetingContent(); got != "早上好，欢迎回来。" {
		t.Fatalf("unexpected chat greeting content: %q", got)
	}

	reloaded, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}
	if got := reloaded.GetLastSleepReviewDate(); got != "2026-02-14" {
		t.Fatalf("unexpected reloaded sleep review date: %q", got)
	}
	if got := reloaded.GetLastWakePlanDate(); got != "2026-02-14" {
		t.Fatalf("unexpected reloaded wake plan date: %q", got)
	}
	if got := reloaded.GetLastPromptEvolutionDate(); got != "2026-02-14" {
		t.Fatalf("unexpected reloaded prompt evolution date: %q", got)
	}
	if got := reloaded.GetLastChatGreetingDate(); got != "2026-02-14" {
		t.Fatalf("unexpected reloaded chat greeting date: %q", got)
	}
	gotReloadedAt := reloaded.GetLastChatGreetingAt()
	if gotReloadedAt.IsZero() || !gotReloadedAt.Equal(greetAt.UTC()) {
		t.Fatalf("unexpected reloaded chat greeting time: %v", gotReloadedAt)
	}
	if got := reloaded.GetLastChatGreetingContent(); got != "早上好，欢迎回来。" {
		t.Fatalf("unexpected reloaded chat greeting content: %q", got)
	}
}

func TestStoreAgentHabitState_InvalidDateRejected(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.SetLastSleepReviewDate("2026/02/14"); err == nil {
		t.Fatalf("expected invalid date to be rejected")
	}
	if err := store.SetLastChatGreetingState("2026/02/14", time.Now(), "x"); err == nil {
		t.Fatalf("expected invalid chat greeting date to be rejected")
	}
	if err := store.SetLastChatGreetingState("2026-02-14", time.Time{}, "ok"); err != nil {
		t.Fatalf("expected empty time to be accepted, got %v", err)
	}
}

func TestStoreLoad_InvalidGreetingTimestampRejected(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	raw := `{
  "agent": {
    "habits": {
      "last_chat_greeting_date": "2026-02-14",
      "last_chat_greeting_at": "2026/02/14 08:30:00"
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("write settings file error: %v", err)
	}

	if _, err := NewStore(settingsPath); err == nil || !strings.Contains(err.Error(), "last_chat_greeting_at") {
		t.Fatalf("expected invalid timestamp error, got %v", err)
	}
}
