package mcp

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

func (s *Store) ListSkills() []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.Skills.Items)
}

func (s *Store) ListEnabledSkillPrompts() []string {
	skills := s.ListSkills()
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		prompt := strings.TrimSpace(skill.Prompt)
		if prompt == "" {
			continue
		}
		out = append(out, prompt)
	}
	return out
}

func (s *Store) ListEnabledSkillIndex() []string {
	skills := s.ListSkills()
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		id := strings.TrimSpace(skill.ID)
		name := strings.TrimSpace(skill.Name)
		description := strings.TrimSpace(skill.Description)
		prompt := strings.TrimSpace(skill.Prompt)
		if id == "" || name == "" || prompt == "" {
			continue
		}
		if description == "" {
			description = normalizeSkillDescription("", name, prompt)
		}
		out = append(out, fmt.Sprintf(
			"skill_id=%s | name=%s | description=%s",
			id,
			name,
			description,
		))
	}
	return out
}

func (s *Store) ReadEnabledSkillPrompt(skillID string) (string, bool) {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return "", false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Prefer exact ID; fallback to unique name match for model robustness.
	for _, skill := range s.cfg.Skills.Items {
		if !skill.Enabled {
			continue
		}
		if strings.TrimSpace(skill.ID) == skillID {
			markdown := renderSkillMarkdown(skill)
			return markdown, strings.TrimSpace(markdown) != ""
		}
	}

	matchedSkill := Skill{}
	matched := false
	for _, skill := range s.cfg.Skills.Items {
		if !skill.Enabled {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(skill.Name), skillID) {
			continue
		}
		prompt := strings.TrimSpace(skill.Prompt)
		if prompt == "" {
			continue
		}
		if matched {
			return "", false
		}
		matchedSkill = skill
		matched = true
	}
	if !matched {
		return "", false
	}
	markdown := renderSkillMarkdown(matchedSkill)
	return markdown, strings.TrimSpace(markdown) != ""
}

func (s *Store) UpsertAutoSkill(name, prompt string) error {
	name = trimSkillText(name, maxAutoSkillNameRunes)
	prompt = trimSkillText(prompt, maxAutoSkillPromptRunes)
	if name == "" {
		return fmt.Errorf("auto skill name is required")
	}
	if prompt == "" {
		return fmt.Errorf("auto skill prompt is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.findAutoSkillIDByNameLocked(name)
	if id == "" {
		id = s.generateAutoSkillIDLocked(name, prompt)
	}

	skill := Skill{
		ID:          id,
		Name:        name,
		Description: normalizeSkillDescription("", name, prompt),
		Prompt:      prompt,
		Enabled:     true,
	}
	if err := validateSkill(skill); err != nil {
		return err
	}

	skill.UpdatedAt = time.Now()
	updated := false
	for i := range s.cfg.Skills.Items {
		if s.cfg.Skills.Items[i].ID == skill.ID {
			s.cfg.Skills.Items[i] = skill
			updated = true
			break
		}
	}
	if !updated {
		s.cfg.Skills.Items = append(s.cfg.Skills.Items, skill)
	}

	s.trimAutoSkillsLocked(maxAutoSkillsRetained)
	return s.persistLocked()
}

func (s *Store) UpsertSkill(skill Skill) error {
	skill.ID = strings.TrimSpace(skill.ID)
	skill.Name = strings.TrimSpace(skill.Name)
	skill.Description = normalizeSkillDescription(skill.Description, skill.Name, skill.Prompt)
	skill.Prompt = strings.TrimSpace(skill.Prompt)
	s.mu.Lock()
	defer s.mu.Unlock()

	if skill.ID == "" {
		skill.ID = s.findSkillIDForUpdateLocked(skill)
	}
	if skill.ID == "" {
		skill.ID = generateUniqueSkillID(s.cfg.Skills.Items, skill.Name, skill.Prompt)
	}
	if skill.Name == "" {
		skill.Name = skill.ID
	}
	skill.Description = normalizeSkillDescription(skill.Description, skill.Name, skill.Prompt)
	if err := validateSkill(skill); err != nil {
		return err
	}

	skill.UpdatedAt = time.Now()
	updated := false
	for i := range s.cfg.Skills.Items {
		if s.cfg.Skills.Items[i].ID == skill.ID {
			s.cfg.Skills.Items[i] = skill
			updated = true
			break
		}
	}
	if !updated {
		s.cfg.Skills.Items = append(s.cfg.Skills.Items, skill)
	}

	return s.persistLocked()
}

func (s *Store) DeleteSkill(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skill id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := make([]Skill, 0, len(s.cfg.Skills.Items))
	for _, skill := range s.cfg.Skills.Items {
		if skill.ID != id {
			next = append(next, skill)
		}
	}
	s.cfg.Skills.Items = next
	return s.persistLocked()
}

func (s *Store) SetSkillEnabled(id string, enabled bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skill id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i := range s.cfg.Skills.Items {
		if s.cfg.Skills.Items[i].ID == id {
			s.cfg.Skills.Items[i].Enabled = enabled
			s.cfg.Skills.Items[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("skill %q not found", id)
	}
	return s.persistLocked()
}
