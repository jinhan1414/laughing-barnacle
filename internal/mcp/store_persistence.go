package mcp

import (
	"encoding/json"
	"fmt"
	"laughing-barnacle/internal/fileutil"
	"os"
	"path/filepath"
	"strings"
)

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.cfg = fileConfig{}
			s.cfg.Agent.Prompts = DefaultAgentPromptConfig()
			s.cfg.Agent.Schedules = nil
			return s.persistLocked()
		}
		return fmt.Errorf("read settings file: %w", err)
	}

	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("decode settings file: %w", err)
	}
	needsPersist := false
	for i, svc := range cfg.MCP.Services {
		svc.Transport = normalizeServiceTransport(svc.Transport)
		svc.Command = strings.TrimSpace(svc.Command)
		svc.Args = normalizeServiceArgs(svc.Args)
		svc.ToolStates = normalizeServiceToolStates(svc.ToolStates)
		if err := validateService(svc); err != nil {
			return fmt.Errorf("invalid mcp service %q: %w", svc.ID, err)
		}
		cfg.MCP.Services[i] = svc
	}
	for i, skill := range cfg.Skills.Items {
		skill.ID = strings.TrimSpace(skill.ID)
		skill.Name = strings.TrimSpace(skill.Name)
		skill.Prompt = strings.TrimSpace(skill.Prompt)
		nextDescription := normalizeSkillDescription(skill.Description, skill.Name, skill.Prompt)
		if strings.TrimSpace(skill.Description) != nextDescription {
			skill.Description = nextDescription
			needsPersist = true
		}
		if err := validateSkill(skill); err != nil {
			return fmt.Errorf("invalid skill %q: %w", skill.ID, err)
		}
		cfg.Skills.Items[i] = skill
	}
	if normalizedPrompts, changed := normalizeAgentPromptConfigOnLoad(cfg.Agent.Prompts); changed {
		cfg.Agent.Prompts = normalizedPrompts
		needsPersist = true
	}
	if err := validateAgentPromptConfig(cfg.Agent.Prompts); err != nil {
		return fmt.Errorf("invalid agent prompts: %w", err)
	}
	if err := validateAgentHabitState(cfg.Agent.Habits); err != nil {
		return fmt.Errorf("invalid agent habits: %w", err)
	}
	if strings.TrimSpace(cfg.Agent.Prompts.SystemPrompt) == "" &&
		strings.TrimSpace(cfg.Agent.Prompts.CompressionSystemPrompt) == "" {
		cfg.Agent.Prompts = DefaultAgentPromptConfig()
		needsPersist = true
	}
	normalizedSchedules, changed, err := normalizeAndMergeScheduledTasks(cfg.Agent.Schedules)
	if err != nil {
		return fmt.Errorf("invalid scheduled tasks: %w", err)
	}
	cfg.Agent.Schedules = normalizedSchedules
	if changed {
		needsPersist = true
	}

	s.cfg = cfg
	if needsPersist {
		return s.persistLocked()
	}
	return nil
}

func (s *Store) persistLocked() error {
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}

	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp settings: %w", err)
	}
	if err := fileutil.ReplaceFileWithRetry(tempPath, s.path); err != nil {
		return fmt.Errorf("rename settings file: %w", err)
	}
	return nil
}
