package skills

import (
	"fmt"
	"strings"
)

func validateSkillID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skill id is required")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("skill id must match [a-zA-Z0-9_-]+")
	}
	return nil
}

func findSkillIDForUpdate(existing []Skill, skill Skill) string {
	name := strings.TrimSpace(skill.Name)
	if name == "" {
		return ""
	}
	matched := ""
	for _, item := range existing {
		if !strings.EqualFold(strings.TrimSpace(item.Name), name) {
			continue
		}
		if matched != "" {
			return ""
		}
		matched = item.ID
	}
	return matched
}

func findAutoSkillIDByName(existing []Skill, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for _, skill := range existing {
		if !strings.HasPrefix(strings.TrimSpace(skill.ID), autoSkillIDPrefix) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(skill.Name), name) {
			return skill.ID
		}
	}
	return ""
}

func generateAutoSkillID(existing []Skill, name, prompt string) string {
	seed := sanitizeIdentifier(name)
	if seed == "" {
		seed = sanitizeIdentifier(prompt)
	}
	if seed == "" {
		seed = "skill"
	}

	candidate := autoSkillIDPrefix + seed
	used := make(map[string]struct{}, len(existing))
	for _, skill := range existing {
		used[skill.ID] = struct{}{}
	}
	if _, ok := used[candidate]; !ok {
		return candidate
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s%s-%d", autoSkillIDPrefix, seed, i)
		if _, ok := used[next]; !ok {
			return next
		}
	}
}

func generateUniqueSkillID(existing []Skill, name, prompt string) string {
	used := make(map[string]struct{}, len(existing))
	for _, skill := range existing {
		used[skill.ID] = struct{}{}
	}
	base := sanitizeIdentifier(name)
	if base == "" {
		base = sanitizeIdentifier(prompt)
	}
	if base == "" {
		base = "skill"
	}
	if _, ok := used[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s-%d", base, i)
		if _, ok := used[next]; !ok {
			return next
		}
	}
}

func sanitizeIdentifier(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteRune('-')
			lastDash = true
		default:
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func trimSkillText(v string, max int) string {
	v = strings.TrimSpace(v)
	if v == "" || max <= 0 {
		return ""
	}
	runes := []rune(v)
	if len(runes) <= max {
		return v
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return strings.TrimSpace(string(runes[:max-3])) + "..."
}

func normalizeSkillDescription(description, name, prompt string) string {
	description = strings.TrimSpace(strings.ReplaceAll(description, "\n", " "))
	if description != "" {
		return trimSkillText(description, 140)
	}

	base := strings.TrimSpace(prompt)
	if base == "" {
		base = strings.TrimSpace(name)
	}
	if base == "" {
		return ""
	}
	base = strings.ReplaceAll(base, "\n", " ")
	return trimSkillText(base, 140)
}

func splitPathSegments(path string) []string {
	trimmed := strings.Trim(path, " /")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
