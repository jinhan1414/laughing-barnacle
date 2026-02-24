package skills

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func (s *Store) trimAutoSkillsLocked(limit int) {
	skills, err := s.listSkillsLocked()
	if err != nil {
		return
	}
	autos := make([]Skill, 0)
	for _, skill := range skills {
		if strings.HasPrefix(strings.TrimSpace(skill.ID), autoSkillIDPrefix) {
			autos = append(autos, skill)
		}
	}
	if len(autos) <= limit {
		return
	}

	sort.Slice(autos, func(i, j int) bool {
		if autos[i].UpdatedAt.Equal(autos[j].UpdatedAt) {
			return autos[i].ID < autos[j].ID
		}
		return autos[i].UpdatedAt.Before(autos[j].UpdatedAt)
	})

	removeCount := len(autos) - limit
	for i := 0; i < removeCount; i++ {
		id := autos[i].ID
		_ = os.RemoveAll(filepath.Join(s.dir, id))
		delete(s.state.Skills, id)
	}
}

func parseSkillMarkdown(markdown string) (name, description, prompt string) {
	text := strings.TrimSpace(strings.ReplaceAll(markdown, "\r\n", "\n"))
	if text == "" {
		return "", "", ""
	}
	if !strings.HasPrefix(text, "---\n") {
		return "", "", text
	}

	rest := strings.TrimPrefix(text, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", "", text
	}
	header := rest[:idx]
	body := strings.TrimSpace(rest[idx+5:])

	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			if unquoted, err := strconv.Unquote(value); err == nil {
				value = unquoted
			}
		}
		switch key {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return strings.TrimSpace(name), strings.TrimSpace(description), body
}

func renderSkillMarkdown(skill Skill) string {
	name := strings.TrimSpace(skill.Name)
	if name == "" {
		name = strings.TrimSpace(skill.ID)
	}
	description := normalizeSkillDescription(skill.Description, name, skill.Prompt)
	return strings.TrimSpace(
		"---\n" +
			"name: " + quoteYAMLString(name) + "\n" +
			"description: " + quoteYAMLString(description) + "\n" +
			"---\n\n" +
			strings.TrimSpace(skill.Prompt),
	)
}

func quoteYAMLString(v string) string {
	return strconv.Quote(strings.TrimSpace(strings.ReplaceAll(v, "\n", " ")))
}
