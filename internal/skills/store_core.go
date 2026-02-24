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
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return "", false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	skills, err := s.listSkillsLocked()
	if err != nil {
		return "", false
	}

	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		if strings.TrimSpace(skill.ID) != skillID {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, skill.ID, "SKILL.md"))
		if err != nil {
			return "", false
		}
		markdown := strings.TrimSpace(string(data))
		return markdown, markdown != ""
	}

	matched := Skill{}
	found := false
	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(skill.Name), skillID) {
			continue
		}
		if found {
			return "", false
		}
		matched = skill
		found = true
	}
	if !found {
		return "", false
	}

	data, err := os.ReadFile(filepath.Join(s.dir, matched.ID, "SKILL.md"))
	if err != nil {
		return "", false
	}
	markdown := strings.TrimSpace(string(data))
	return markdown, markdown != ""
}

func (s *Store) UpsertSkill(skill Skill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertSkillLocked(skill)
}

func (s *Store) upsertSkillLocked(skill Skill) error {
	skills, err := s.listSkillsLocked()
	if err != nil {
		return err
	}

	skill.ID = strings.TrimSpace(skill.ID)
	skill.Name = strings.TrimSpace(skill.Name)
	skill.Prompt = strings.TrimSpace(skill.Prompt)
	skill.Description = normalizeSkillDescription(skill.Description, skill.Name, skill.Prompt)

	if skill.ID == "" {
		skill.ID = findSkillIDForUpdate(skills, skill)
	}
	if skill.ID == "" {
		skill.ID = generateUniqueSkillID(skills, skill.Name, skill.Prompt)
	}
	if skill.Name == "" {
		skill.Name = skill.ID
	}
	if skill.Description == "" {
		skill.Description = normalizeSkillDescription("", skill.Name, skill.Prompt)
	}
	if strings.TrimSpace(skill.Prompt) == "" {
		return fmt.Errorf("skill prompt is required")
	}
	if err := validateSkillID(skill.ID); err != nil {
		return err
	}

	now := time.Now()
	markdown := renderSkillMarkdown(skill)
	dirPath := filepath.Join(s.dir, skill.ID)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "SKILL.md"), []byte(markdown+"\n"), 0o600); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}

	record := s.state.Skills[skill.ID]
	record.Enabled = skill.Enabled
	record.UpdatedAt = now
	if src := strings.TrimSpace(skill.Source); src != "" {
		record.Source = src
	}
	s.state.Skills[skill.ID] = record
	return s.persistLocked()
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
