package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	ErrSkillNotFound            = errors.New("skill not found or not enabled")
	ErrSkillResourceNotFound    = errors.New("skill resource not found")
	ErrInvalidSkillResourcePath = errors.New("invalid skill resource path")
)

type SkillResource struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Readable   bool   `json:"readable"`
	Executable bool   `json:"executable,omitempty"`
}

type SkillResourceIndex struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Resources []SkillResource `json:"resources"`
}

func (s *Store) ReadEnabledSkillResource(skillID, resourcePath string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	skill, err := s.resolveEnabledSkillLocked(skillID)
	if err != nil {
		return "", err
	}
	normalizedPath, err := normalizeSkillResourcePath(resourcePath)
	if err != nil {
		return "", err
	}
	data, readErr := os.ReadFile(filepath.Join(s.dir, skill.ID, filepath.FromSlash(normalizedPath)))
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return "", ErrSkillResourceNotFound
		}
		return "", readErr
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", ErrSkillResourceNotFound
	}
	return content, nil
}

func (s *Store) ReadEnabledSkillResourceIndex(skillID string) (SkillResourceIndex, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	skill, err := s.resolveEnabledSkillLocked(skillID)
	if err != nil {
		return SkillResourceIndex{}, err
	}
	root := filepath.Join(s.dir, skill.ID)
	resources := []SkillResource{{
		Path:     "SKILL.md",
		Kind:     "skill",
		Readable: true,
	}}
	resources = append(resources, collectSkillResources(root, "references", "reference", true, false)...)
	resources = append(resources, collectSkillResources(root, "scripts", "script", false, true)...)
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Path < resources[j].Path
	})
	return SkillResourceIndex{
		ID:        skill.ID,
		Name:      skill.Name,
		Resources: resources,
	}, nil
}

func (s *Store) resolveEnabledSkillLocked(skillID string) (Skill, error) {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return Skill{}, ErrSkillNotFound
	}
	skills, err := s.listSkillsLocked()
	if err != nil {
		return Skill{}, err
	}
	for _, skill := range skills {
		if skill.Enabled && strings.TrimSpace(skill.ID) == skillID {
			return skill, nil
		}
	}
	matched := Skill{}
	found := false
	for _, skill := range skills {
		if !skill.Enabled || !strings.EqualFold(strings.TrimSpace(skill.Name), skillID) {
			continue
		}
		if found {
			return Skill{}, ErrSkillNotFound
		}
		matched = skill
		found = true
	}
	if !found {
		return Skill{}, ErrSkillNotFound
	}
	return matched, nil
}

func normalizeSkillResourcePath(resourcePath string) (string, error) {
	path := strings.TrimSpace(strings.ReplaceAll(resourcePath, "\\", "/"))
	if path == "" {
		return "SKILL.md", nil
	}
	if strings.HasPrefix(path, "/") {
		return "", ErrInvalidSkillResourcePath
	}
	segments := splitPathSegments(path)
	if len(segments) == 0 {
		return "", ErrInvalidSkillResourcePath
	}
	for _, segment := range segments {
		if segment == "." || segment == ".." {
			return "", ErrInvalidSkillResourcePath
		}
	}
	normalized := strings.Join(segments, "/")
	if normalized == "SKILL.md" {
		return normalized, nil
	}
	if strings.HasPrefix(normalized, "references/") && strings.HasSuffix(strings.ToLower(normalized), ".md") {
		return normalized, nil
	}
	return "", fmt.Errorf("%w: %s", ErrInvalidSkillResourcePath, normalized)
}

func collectSkillResources(root, subdir, kind string, readable, executable bool) []SkillResource {
	target := filepath.Join(root, subdir)
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return nil
	}
	resources := make([]SkillResource, 0)
	_ = filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if kind == "reference" && !strings.HasSuffix(strings.ToLower(rel), ".md") {
			return nil
		}
		resources = append(resources, SkillResource{
			Path:       rel,
			Kind:       kind,
			Readable:   readable,
			Executable: executable,
		})
		return nil
	})
	return resources
}
