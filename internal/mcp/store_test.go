package mcp

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreUpsertAndReload(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertService(Service{
		ID:        "search",
		Name:      "Search",
		Endpoint:  "https://example.com/mcp",
		AuthToken: "token-1",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService error: %v", err)
	}

	if err := store.UpsertService(Service{
		ID:       "search",
		Name:     "Search API",
		Endpoint: "https://example.com/mcp",
		Enabled:  false,
	}); err != nil {
		t.Fatalf("UpsertService update error: %v", err)
	}

	svc, ok := store.GetService("search")
	if !ok {
		t.Fatalf("service not found")
	}
	if svc.AuthToken != "token-1" {
		t.Fatalf("expected auth token to be preserved, got %q", svc.AuthToken)
	}
	if svc.Enabled {
		t.Fatalf("expected service disabled")
	}

	reloaded, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}

	services := reloaded.ListServices()
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	if services[0].Name != "Search API" {
		t.Fatalf("unexpected service name: %q", services[0].Name)
	}
	if services[0].AuthToken != "token-1" {
		t.Fatalf("unexpected token after reload: %q", services[0].AuthToken)
	}
}

func TestStoreSkillCRUDAndPrompts(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		ID:      "research",
		Name:    "Research Skill",
		Prompt:  "先检索再回答，给出来源。",
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertSkill error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		ID:      "research",
		Name:    "Research Skill v2",
		Prompt:  "先检索、再总结、最后给出来源。",
		Enabled: false,
	}); err != nil {
		t.Fatalf("UpsertSkill update error: %v", err)
	}

	if err := store.SetSkillEnabled("research", true); err != nil {
		t.Fatalf("SetSkillEnabled error: %v", err)
	}

	prompts := store.ListEnabledSkillPrompts()
	if len(prompts) != 1 {
		t.Fatalf("expected 1 enabled prompt, got %d", len(prompts))
	}
	if prompts[0] != "先检索、再总结、最后给出来源。" {
		t.Fatalf("unexpected prompt: %q", prompts[0])
	}

	if err := store.DeleteSkill("research"); err != nil {
		t.Fatalf("DeleteSkill error: %v", err)
	}
	if len(store.ListSkills()) != 0 {
		t.Fatalf("expected no skills after delete")
	}
}

func TestStoreUpsertAutoSkill_PersistedAndBounded(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertAutoSkill("复盘框架", "先列事实、再列根因、最后列行动项。"); err != nil {
		t.Fatalf("UpsertAutoSkill error: %v", err)
	}
	if err := store.UpsertAutoSkill("复盘框架", "先列事实时间线，再写根因和防复发动作。"); err != nil {
		t.Fatalf("UpsertAutoSkill update error: %v", err)
	}

	skills := store.ListSkills()
	if len(skills) != 1 {
		t.Fatalf("expected one auto skill, got %d", len(skills))
	}
	if !strings.HasPrefix(skills[0].ID, autoSkillIDPrefix) {
		t.Fatalf("expected auto skill id prefix, got %q", skills[0].ID)
	}
	if !skills[0].Enabled {
		t.Fatalf("expected auto skill enabled")
	}
	if !strings.Contains(skills[0].Prompt, "防复发") {
		t.Fatalf("expected updated auto skill prompt, got %q", skills[0].Prompt)
	}

	for i := 0; i < maxAutoSkillsRetained+3; i++ {
		name := fmt.Sprintf("自动能力-%d", i)
		prompt := fmt.Sprintf("这是第 %d 条自动能力，用于验证上限裁剪。", i)
		if err := store.UpsertAutoSkill(name, prompt); err != nil {
			t.Fatalf("UpsertAutoSkill #%d error: %v", i, err)
		}
	}

	autoCount := 0
	for _, skill := range store.ListSkills() {
		if strings.HasPrefix(skill.ID, autoSkillIDPrefix) {
			autoCount++
		}
	}
	if autoCount != maxAutoSkillsRetained {
		t.Fatalf("expected auto skill count capped to %d, got %d", maxAutoSkillsRetained, autoCount)
	}
}

func TestStoreUpsertService_AutoGeneratesIDWhenMissing(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertService(Service{
		Name:     "Search MCP",
		Endpoint: "https://example.com/mcp/search",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertService error: %v", err)
	}
	if err := store.UpsertService(Service{
		Name:     "Search MCP",
		Endpoint: "https://example.com/mcp/search2",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("UpsertService second insert error: %v", err)
	}

	services := store.ListServices()
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
	if services[0].ID == "" || services[1].ID == "" {
		t.Fatalf("expected generated ids, got %+v", services)
	}
	if services[0].ID == services[1].ID {
		t.Fatalf("expected unique generated ids, got %q", services[0].ID)
	}
}

func TestStoreUpsertSkill_AutoGeneratesIDWhenMissing(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		Name:    "Research Mode",
		Prompt:  "先检索再回答",
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertSkill error: %v", err)
	}
	if err := store.UpsertSkill(Skill{
		Name:    "Writing Mode",
		Prompt:  "先检索再回答，附来源",
		Enabled: false,
	}); err != nil {
		t.Fatalf("UpsertSkill second insert error: %v", err)
	}

	skills := store.ListSkills()
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}
	if skills[0].ID == "" || skills[1].ID == "" {
		t.Fatalf("expected generated ids, got %+v", skills)
	}
	if skills[0].ID == skills[1].ID {
		t.Fatalf("expected unique generated ids, got %q", skills[0].ID)
	}
}

func TestStoreUpsertSkill_EmptyIDUpdatesExistingByName(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		ID:      "research",
		Name:    "Research Mode",
		Prompt:  "先检索",
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertSkill error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		Name:    "Research Mode",
		Prompt:  "先检索再回答并附来源",
		Enabled: false,
	}); err != nil {
		t.Fatalf("UpsertSkill update error: %v", err)
	}

	skills := store.ListSkills()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill after update, got %d", len(skills))
	}
	if skills[0].ID != "research" {
		t.Fatalf("expected id keep research, got %q", skills[0].ID)
	}
	if skills[0].Prompt != "先检索再回答并附来源" {
		t.Fatalf("expected prompt updated, got %q", skills[0].Prompt)
	}
	if skills[0].Enabled {
		t.Fatalf("expected skill disabled after update")
	}
}

func TestStoreUpsertService_EmptyIDUpdatesExistingByEndpoint(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertService(Service{
		ID:        "deepwiki",
		Name:      "DeepWiki",
		Endpoint:  "https://mcp.deepwiki.com/mcp",
		Transport: "streamable_http",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService error: %v", err)
	}

	if err := store.UpsertService(Service{
		Name:      "DeepWiki PROD",
		Endpoint:  "https://mcp.deepwiki.com/mcp",
		Transport: "streamableHttp",
		Enabled:   false,
	}); err != nil {
		t.Fatalf("UpsertService update error: %v", err)
	}

	services := store.ListServices()
	if len(services) != 1 {
		t.Fatalf("expected 1 service after update, got %d", len(services))
	}
	if services[0].ID != "deepwiki" {
		t.Fatalf("expected id keep deepwiki, got %q", services[0].ID)
	}
	if services[0].Name != "DeepWiki PROD" {
		t.Fatalf("expected name updated, got %q", services[0].Name)
	}
	if services[0].Enabled {
		t.Fatalf("expected service disabled after update")
	}
	if services[0].Transport != ServiceTransportStreamableHTTP {
		t.Fatalf("expected normalized streamable transport, got %q", services[0].Transport)
	}
}

func TestStoreSetServiceToolEnabled_Persisted(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertService(Service{
		ID:        "search",
		Name:      "Search",
		Endpoint:  "https://example.com/mcp",
		Transport: "streamable_http",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpsertService error: %v", err)
	}

	if !store.IsServiceToolEnabled("search", "web_search") {
		t.Fatalf("tool should be enabled by default")
	}
	if err := store.SetServiceToolEnabled("search", "web_search", false); err != nil {
		t.Fatalf("SetServiceToolEnabled disable error: %v", err)
	}
	if store.IsServiceToolEnabled("search", "web_search") {
		t.Fatalf("tool should be disabled")
	}

	if err := store.SetServiceToolEnabled("search", "web_search", true); err != nil {
		t.Fatalf("SetServiceToolEnabled enable error: %v", err)
	}
	if !store.IsServiceToolEnabled("search", "web_search") {
		t.Fatalf("tool should be enabled after toggle back")
	}

	if err := store.SetServiceToolEnabled("search", "weather", false); err != nil {
		t.Fatalf("SetServiceToolEnabled second tool disable error: %v", err)
	}

	reloaded, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("reload store error: %v", err)
	}
	if !reloaded.IsServiceToolEnabled("search", "web_search") {
		t.Fatalf("web_search should stay enabled after reload")
	}
	if reloaded.IsServiceToolEnabled("search", "weather") {
		t.Fatalf("weather should stay disabled after reload")
	}
}

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

	if got := store.GetLastSleepReviewDate(); got != "2026-02-14" {
		t.Fatalf("unexpected sleep review date: %q", got)
	}
	if got := store.GetLastWakePlanDate(); got != "2026-02-14" {
		t.Fatalf("unexpected wake plan date: %q", got)
	}
	if got := store.GetLastPromptEvolutionDate(); got != "2026-02-14" {
		t.Fatalf("unexpected prompt evolution date: %q", got)
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
}
