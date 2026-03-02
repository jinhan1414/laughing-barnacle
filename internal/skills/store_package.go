package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) UpsertSkillPackage(pkg SkillPackage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertSkillPackageLocked(pkg)
}

func (s *Store) upsertSkillPackageLocked(pkg SkillPackage) error {
	skills, err := s.listSkillsLocked()
	if err != nil {
		return err
	}

	skill, err := prepareSkillForSave(skills, pkg.Skill)
	if err != nil {
		return err
	}
	if err := s.replaceSkillDir(skill, pkg.Resources); err != nil {
		return err
	}

	record := s.state.Skills[skill.ID]
	record.Enabled = skill.Enabled
	record.UpdatedAt = time.Now()
	if src := strings.TrimSpace(skill.Source); src != "" {
		record.Source = src
	}
	s.state.Skills[skill.ID] = record
	return s.persistLocked()
}

func prepareSkillForSave(skills []Skill, skill Skill) (Skill, error) {
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
		return Skill{}, fmt.Errorf("skill prompt is required")
	}
	if err := validateSkillID(skill.ID); err != nil {
		return Skill{}, err
	}
	return skill, nil
}

func (s *Store) replaceSkillDir(skill Skill, resources []SkillPackageResource) error {
	tmpDir, err := os.MkdirTemp(s.dir, skill.ID+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp skill dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := writeSkillPackageFiles(tmpDir, skill, resources); err != nil {
		return err
	}

	targetDir := filepath.Join(s.dir, skill.ID)
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("clear existing skill dir: %w", err)
	}
	if err := os.Rename(tmpDir, targetDir); err != nil {
		return fmt.Errorf("activate skill dir: %w", err)
	}
	return nil
}

func writeSkillPackageFiles(root string, skill Skill, resources []SkillPackageResource) error {
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(renderSkillMarkdown(skill)+"\n"), 0o600); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}
	for _, resource := range resources {
		path, err := normalizeSkillWriteResourcePath(resource.Path)
		if err != nil {
			return err
		}
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("create skill resource dir: %w", err)
		}
		if err := os.WriteFile(fullPath, []byte(resource.Content), 0o600); err != nil {
			return fmt.Errorf("write skill resource: %w", err)
		}
	}
	return nil
}
