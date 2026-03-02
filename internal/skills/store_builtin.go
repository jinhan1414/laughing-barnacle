package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type builtinSkillPackage struct {
	Skill     Skill
	SourceDir string
}

func defaultBuiltinSkillsDir() string {
	candidates := []string{
		filepath.Clean("./builtin-skills"),
	}
	if _, currentFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "builtin-skills")))
	}
	for _, candidate := range candidates {
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate
		}
	}
	return filepath.Clean("./builtin-skills")
}

func (s *Store) ensureBuiltinSkillsLocked() error {
	if s.state.Skills == nil {
		s.state.Skills = map[string]skillStateRecord{}
	}

	packages, err := s.loadBuiltinSkillPackagesLocked()
	if err != nil {
		return err
	}

	stateChanged := false
	validIDs := make(map[string]struct{}, len(packages))
	for _, pkg := range packages {
		validIDs[pkg.Skill.ID] = struct{}{}
		changed, syncErr := s.syncBuiltinSkillPackageLocked(pkg)
		if syncErr != nil {
			return syncErr
		}
		stateChanged = stateChanged || changed
	}

	pruned, err := s.pruneMissingBuiltinSkillsLocked(validIDs)
	if err != nil {
		return err
	}
	stateChanged = stateChanged || pruned
	if !stateChanged {
		return nil
	}
	return s.persistLocked()
}

func (s *Store) loadBuiltinSkillPackagesLocked() ([]builtinSkillPackage, error) {
	root := strings.TrimSpace(s.builtinDir)
	if root == "" {
		return nil, fmt.Errorf("builtin skills directory is required")
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read builtin skills directory: %w", err)
	}

	packages := make([]builtinSkillPackage, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillID := strings.TrimSpace(entry.Name())
		if err := validateSkillID(skillID); err != nil {
			return nil, fmt.Errorf("invalid builtin skill id %q: %w", skillID, err)
		}

		sourceDir := filepath.Join(root, skillID)
		skillPath := filepath.Join(sourceDir, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("builtin skill %q missing SKILL.md", skillID)
			}
			return nil, fmt.Errorf("read builtin skill %q: %w", skillID, err)
		}

		name, description, prompt := parseSkillMarkdown(string(data))
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("builtin skill %q name is required", skillID)
		}
		if strings.TrimSpace(description) == "" {
			return nil, fmt.Errorf("builtin skill %q description is required", skillID)
		}
		if strings.TrimSpace(prompt) == "" {
			return nil, fmt.Errorf("builtin skill %q prompt is required", skillID)
		}

		packages = append(packages, builtinSkillPackage{
			Skill: Skill{
				ID:          skillID,
				Name:        strings.TrimSpace(name),
				Description: strings.TrimSpace(description),
				Prompt:      strings.TrimSpace(prompt),
				Enabled:     true,
				Source:      builtinSkillSource,
			},
			SourceDir: sourceDir,
		})
	}

	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Skill.ID < packages[j].Skill.ID
	})
	return packages, nil
}

func (s *Store) syncBuiltinSkillPackageLocked(pkg builtinSkillPackage) (bool, error) {
	record, exists := s.state.Skills[pkg.Skill.ID]
	source := strings.TrimSpace(record.Source)
	if exists && source != "" && !strings.EqualFold(source, builtinSkillSource) {
		return false, fmt.Errorf("builtin skill id conflicts with non-builtin skill: %s", pkg.Skill.ID)
	}

	stateChanged := false
	if !exists {
		record = skillStateRecord{
			Enabled:   true,
			Source:    builtinSkillSource,
			UpdatedAt: time.Now(),
		}
		s.state.Skills[pkg.Skill.ID] = record
		stateChanged = true
	} else if source == "" {
		record.Source = builtinSkillSource
		s.state.Skills[pkg.Skill.ID] = record
		stateChanged = true
	}

	dstDir := filepath.Join(s.dir, pkg.Skill.ID)
	if err := os.RemoveAll(dstDir); err != nil {
		return stateChanged, fmt.Errorf("reset builtin skill dir %q: %w", pkg.Skill.ID, err)
	}
	if err := copyDir(pkg.SourceDir, dstDir); err != nil {
		return stateChanged, fmt.Errorf("sync builtin skill %q: %w", pkg.Skill.ID, err)
	}

	localized := s.withLocalAPIBaseURLLocked(pkg.Skill)
	rendered := renderSkillMarkdown(localized) + "\n"
	if err := os.WriteFile(filepath.Join(dstDir, "SKILL.md"), []byte(rendered), 0o600); err != nil {
		return stateChanged, fmt.Errorf("write builtin skill %q: %w", pkg.Skill.ID, err)
	}
	return stateChanged, nil
}

func (s *Store) pruneMissingBuiltinSkillsLocked(validIDs map[string]struct{}) (bool, error) {
	changed := false
	for id, record := range s.state.Skills {
		if !strings.EqualFold(strings.TrimSpace(record.Source), builtinSkillSource) {
			continue
		}
		if _, ok := validIDs[id]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.dir, id)); err != nil {
			return changed, fmt.Errorf("remove stale builtin skill %q: %w", id, err)
		}
		delete(s.state.Skills, id)
		changed = true
	}
	return changed, nil
}
