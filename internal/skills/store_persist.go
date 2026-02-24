package skills

import (
	"encoding/json"
	"fmt"
	"laughing-barnacle/internal/fileutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create skills directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o755); err != nil {
		return fmt.Errorf("create skills state directory: %w", err)
	}

	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.state = stateFile{Skills: map[string]skillStateRecord{}}
			return s.ensureBuiltinSkillsLocked()
		}
		return fmt.Errorf("read skills state: %w", err)
	}

	if strings.TrimSpace(string(data)) == "" {
		s.state = stateFile{Skills: map[string]skillStateRecord{}}
		return s.ensureBuiltinSkillsLocked()
	}

	var parsed stateFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("decode skills state: %w", err)
	}
	if parsed.Skills == nil {
		parsed.Skills = map[string]skillStateRecord{}
	}
	s.state = parsed
	return s.ensureBuiltinSkillsLocked()
}

func (s *Store) persistLocked() error {
	if s.state.Skills == nil {
		s.state.Skills = map[string]skillStateRecord{}
	}

	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode skills state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o755); err != nil {
		return fmt.Errorf("create skills state directory: %w", err)
	}
	tmpPath := s.statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp skills state: %w", err)
	}
	if err := fileutil.ReplaceFileWithRetry(tmpPath, s.statePath); err != nil {
		return fmt.Errorf("rename skills state: %w", err)
	}
	return nil
}

func (s *Store) ensureBuiltinSkillsLocked() error {
	if s.state.Skills == nil {
		s.state.Skills = map[string]skillStateRecord{}
	}

	changed := false
	for _, builtin := range builtinSkills {
		id := strings.TrimSpace(builtin.ID)
		if id == "" {
			continue
		}

		skillDir := filepath.Join(s.dir, id)
		skillPath := filepath.Join(skillDir, "SKILL.md")
		shouldWriteFile := false
		if _, err := os.Stat(skillPath); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("stat builtin skill %q: %w", id, err)
			}
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				return fmt.Errorf("create builtin skill dir %q: %w", id, err)
			}
			shouldWriteFile = true
		}

		record, exists := s.state.Skills[id]
		if !exists {
			record.Enabled = true
			record.Source = builtinSkillSource
			record.UpdatedAt = time.Now()
			s.state.Skills[id] = record
			changed = true
			shouldWriteFile = true
		}
		if strings.EqualFold(strings.TrimSpace(record.Source), builtinSkillSource) {
			shouldWriteFile = true
		}
		if strings.TrimSpace(record.Source) == "" {
			record.Source = builtinSkillSource
			s.state.Skills[id] = record
			changed = true
		}
		if shouldWriteFile {
			if err := os.WriteFile(skillPath, []byte(renderSkillMarkdown(builtin)+"\n"), 0o600); err != nil {
				return fmt.Errorf("write builtin skill %q: %w", id, err)
			}
			changed = true
		}
	}

	if !changed {
		return nil
	}
	return s.persistLocked()
}

func (s *Store) listSkillsLocked() ([]Skill, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read skills directory: %w", err)
	}

	out := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillID := strings.TrimSpace(entry.Name())
		if skillID == "" {
			continue
		}
		skillPath := filepath.Join(s.dir, skillID, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", skillPath, err)
		}

		name, description, prompt := parseSkillMarkdown(string(data))
		if strings.TrimSpace(name) == "" {
			name = skillID
		}
		description = normalizeSkillDescription(description, name, prompt)

		record, hasRecord := s.state.Skills[skillID]
		enabled := true
		if hasRecord {
			enabled = record.Enabled
		}
		updatedAt := record.UpdatedAt
		if updatedAt.IsZero() {
			if info, statErr := os.Stat(skillPath); statErr == nil {
				updatedAt = info.ModTime()
			}
		}

		out = append(out, Skill{
			ID:          skillID,
			Name:        strings.TrimSpace(name),
			Description: strings.TrimSpace(description),
			Prompt:      strings.TrimSpace(prompt),
			Enabled:     enabled,
			Source:      strings.TrimSpace(record.Source),
			UpdatedAt:   updatedAt,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
