package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) ListSkills() []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()

	skills, err := s.listSkillsLocked()
	if err != nil {
		return nil
	}
	return cloneSkills(skills)
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
		out = append(out, fmt.Sprintf(
			"skill_id=%s | name=%s | description=%s",
			id,
			name,
			trimSkillText(description, 28),
		))
	}
	return out
}

func (s *Store) ReadEnabledSkillPrompt(skillID string) (string, bool) {
	content, err := s.ReadEnabledSkillResource(skillID, "")
	return content, err == nil && strings.TrimSpace(content) != ""
}

func (s *Store) UpsertSkill(skill Skill) error {
	return s.UpsertSkillPackage(SkillPackage{Skill: skill})
}

func (s *Store) upsertSkillLocked(skill Skill) error {
	return s.upsertSkillPackageLocked(SkillPackage{Skill: skill})
}

func (s *Store) DeleteSkill(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skill id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.RemoveAll(filepath.Join(s.dir, id)); err != nil {
		return fmt.Errorf("delete skill dir: %w", err)
	}
	delete(s.state.Skills, id)
	return s.persistLocked()
}

func (s *Store) SetSkillEnabled(id string, enabled bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skill id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(filepath.Join(s.dir, id, "SKILL.md")); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("skill %q not found", id)
		}
		return fmt.Errorf("read skill: %w", err)
	}

	record := s.state.Skills[id]
	record.Enabled = enabled
	record.UpdatedAt = time.Now()
	s.state.Skills[id] = record
	return s.persistLocked()
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

	skills, err := s.listSkillsLocked()
	if err != nil {
		return err
	}

	id := findAutoSkillIDByName(skills, name)
	if id == "" {
		id = generateAutoSkillID(skills, name, prompt)
	}

	if err := s.upsertSkillLocked(Skill{
		ID:          id,
		Name:        name,
		Description: normalizeSkillDescription("", name, prompt),
		Prompt:      prompt,
		Enabled:     true,
		Source:      "auto-evolved",
	}); err != nil {
		return err
	}

	s.trimAutoSkillsLocked(maxAutoSkillsRetained)
	return s.persistLocked()
}
