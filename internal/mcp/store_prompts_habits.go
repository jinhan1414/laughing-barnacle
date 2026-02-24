package mcp

import (
	"strings"
	"time"
)

func (s *Store) GetAgentPromptConfig() AgentPromptConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg := s.cfg.Agent.Prompts
	cfg.SystemPrompt = strings.TrimSpace(cfg.SystemPrompt)
	cfg.CompressionSystemPrompt = strings.TrimSpace(cfg.CompressionSystemPrompt)
	return cfg
}

func (s *Store) GetSystemPrompt() string {
	return s.GetAgentPromptConfig().SystemPrompt
}

func (s *Store) GetCompressionSystemPrompt() string {
	return s.GetAgentPromptConfig().CompressionSystemPrompt
}

func (s *Store) UpsertAgentPromptConfig(cfg AgentPromptConfig) error {
	cfg.SystemPrompt = strings.TrimSpace(cfg.SystemPrompt)
	cfg.CompressionSystemPrompt = strings.TrimSpace(cfg.CompressionSystemPrompt)

	if err := validateAgentPromptConfig(cfg); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg.UpdatedAt = time.Now()
	s.cfg.Agent.Prompts = cfg
	return s.persistLocked()
}

func (s *Store) UpdateAgentPrompts(systemPrompt, compressionSystemPrompt string) error {
	return s.UpsertAgentPromptConfig(AgentPromptConfig{
		SystemPrompt:            strings.TrimSpace(systemPrompt),
		CompressionSystemPrompt: strings.TrimSpace(compressionSystemPrompt),
	})
}

func (s *Store) ResetAgentPromptConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := DefaultAgentPromptConfig()
	cfg.UpdatedAt = time.Now()
	s.cfg.Agent.Prompts = cfg
	return s.persistLocked()
}

func (s *Store) GetLastSleepReviewDate() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.Agent.Habits.LastSleepReviewDate)
}

func (s *Store) GetLastWakePlanDate() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.Agent.Habits.LastWakePlanDate)
}

func (s *Store) GetLastPromptEvolutionDate() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.Agent.Habits.LastPromptEvolutionDate)
}

func (s *Store) GetLastChatGreetingDate() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.Agent.Habits.LastChatGreetingDate)
}

func (s *Store) GetLastChatGreetingAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	raw := strings.TrimSpace(s.cfg.Agent.Habits.LastChatGreetingAt)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func (s *Store) GetLastChatGreetingContent() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.Agent.Habits.LastChatGreetingContent)
}

func (s *Store) SetLastSleepReviewDate(date string) error {
	date = strings.TrimSpace(date)
	if err := validateOptionalDate(date); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Agent.Habits.LastSleepReviewDate = date
	s.cfg.Agent.Habits.UpdatedAt = time.Now()
	return s.persistLocked()
}

func (s *Store) SetLastWakePlanDate(date string) error {
	date = strings.TrimSpace(date)
	if err := validateOptionalDate(date); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Agent.Habits.LastWakePlanDate = date
	s.cfg.Agent.Habits.UpdatedAt = time.Now()
	return s.persistLocked()
}

func (s *Store) SetLastPromptEvolutionDate(date string) error {
	date = strings.TrimSpace(date)
	if err := validateOptionalDate(date); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Agent.Habits.LastPromptEvolutionDate = date
	s.cfg.Agent.Habits.UpdatedAt = time.Now()
	return s.persistLocked()
}

func (s *Store) SetLastChatGreetingState(date string, at time.Time, content string) error {
	date = strings.TrimSpace(date)
	if err := validateOptionalDate(date); err != nil {
		return err
	}
	content = trimSkillText(content, maxChatGreetingContentRunes)

	atText := ""
	if !at.IsZero() {
		at = at.UTC()
		atText = at.Format(time.RFC3339)
	}
	if err := validateOptionalTimestamp(atText); err != nil {
		return err
	}
	if date == "" && !at.IsZero() {
		date = at.In(time.Local).Format("2006-01-02")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cfg.Agent.Habits.LastChatGreetingDate = date
	s.cfg.Agent.Habits.LastChatGreetingAt = atText
	s.cfg.Agent.Habits.LastChatGreetingContent = content
	s.cfg.Agent.Habits.UpdatedAt = time.Now()
	return s.persistLocked()
}
