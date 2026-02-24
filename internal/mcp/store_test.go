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

func TestStoreSkillCatalogIndexAndRead(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(settingsPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		ID:          "code-review-playbook",
		Name:        "代码评审手册",
		Description: "用于代码评审与上线风险治理。",
		Prompt:      "完整技能：先确认验收标准，再检查风险与回滚，最后给出上线建议。",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("UpsertSkill enabled error: %v", err)
	}
	if err := store.UpsertSkill(Skill{
		ID:      "disabled-skill",
		Name:    "禁用技能",
		Prompt:  "这条不该出现在索引里",
		Enabled: false,
	}); err != nil {
		t.Fatalf("UpsertSkill disabled error: %v", err)
	}

	index := store.ListEnabledSkillIndex()
	if len(index) != 1 {
		t.Fatalf("expected 1 enabled skill index line, got %d", len(index))
	}
	if !strings.Contains(index[0], "skill_id=code-review-playbook") {
		t.Fatalf("unexpected skill index line: %q", index[0])
	}
	if !strings.Contains(index[0], "description=用于代码评审与上线风险治理。") {
		t.Fatalf("expected description in skill index line, got %q", index[0])
	}
	if strings.Contains(index[0], "brief=") || strings.Contains(index[0], "path=") {
		t.Fatalf("skill index should keep minimal fields only, got %q", index[0])
	}

	prompt, ok := store.ReadEnabledSkillPrompt("code-review-playbook")
	if !ok {
		t.Fatalf("expected skill prompt by id")
	}
	if !strings.Contains(prompt, "---") || !strings.Contains(prompt, `name: "code-review-playbook"`) {
		t.Fatalf("expected SKILL.md front matter, got %q", prompt)
	}
	if !strings.Contains(prompt, `description: "用于代码评审与上线风险治理。"`) {
		t.Fatalf("expected SKILL.md description front matter, got %q", prompt)
	}
	if !strings.Contains(prompt, "验收标准") {
		t.Fatalf("unexpected skill prompt: %q", prompt)
	}

	if _, ok := store.ReadEnabledSkillPrompt("disabled-skill"); ok {
		t.Fatalf("disabled skill should not be readable")
	}
	if _, ok := store.ReadEnabledSkillPrompt("代码评审手册"); !ok {
		t.Fatalf("expected name fallback read")
	}
}
