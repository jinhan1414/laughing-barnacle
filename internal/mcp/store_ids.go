package mcp

import (
	"fmt"
	"sort"

	"laughing-barnacle/internal/scheduler"
	"strings"
	"time"
)

func generateUniqueServiceID(existing []Service, name, endpoint, command string) string {
	used := make(map[string]struct{}, len(existing))
	for _, svc := range existing {
		used[svc.ID] = struct{}{}
	}
	return generateUniqueID(used, []string{name, endpoint, command}, "service")
}

func generateUniqueSkillID(existing []Skill, name, prompt string) string {
	used := make(map[string]struct{}, len(existing))
	for _, skill := range existing {
		used[skill.ID] = struct{}{}
	}
	return generateUniqueID(used, []string{name, prompt}, "skill")
}

func generateUniqueTaskID(existing []scheduler.Task, name, action string) string {
	used := make(map[string]struct{}, len(existing))
	for _, task := range existing {
		used[task.ID] = struct{}{}
	}
	return generateUniqueID(used, []string{name, action}, "task")
}

func generateUniqueID(used map[string]struct{}, candidates []string, fallback string) string {
	base := ""
	for _, candidate := range candidates {
		base = sanitizeIdentifier(candidate)
		if base != "" {
			break
		}
	}
	if base == "" {
		base = fallback
	}
	if _, exists := used[base]; !exists {
		return base
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s-%d", base, i)
		if _, exists := used[next]; !exists {
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

func normalizeServiceTransport(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "", "streamable_http":
		return ServiceTransportStreamableHTTP
	case "sse":
		return ServiceTransportSSE
	case "stdio":
		return ServiceTransportStdio
	default:
		return normalized
	}
}

func (s *Store) findServiceIDForUpdateLocked(service Service) string {
	if service.Transport == ServiceTransportStdio {
		command := strings.TrimSpace(service.Command)
		if command == "" {
			return ""
		}
		argsKey := strings.Join(service.Args, "\x00")
		for _, existing := range s.cfg.MCP.Services {
			if normalizeServiceTransport(existing.Transport) != ServiceTransportStdio {
				continue
			}
			if strings.TrimSpace(existing.Command) == command &&
				strings.Join(normalizeServiceArgs(existing.Args), "\x00") == argsKey {
				return existing.ID
			}
		}
		return ""
	}

	endpoint := strings.TrimSpace(service.Endpoint)
	if endpoint != "" {
		for _, existing := range s.cfg.MCP.Services {
			if strings.TrimSpace(existing.Endpoint) == endpoint {
				return existing.ID
			}
		}
	}
	return ""
}

func (s *Store) findSkillIDForUpdateLocked(skill Skill) string {
	name := strings.TrimSpace(skill.Name)
	if name == "" {
		return ""
	}

	matchedID := ""
	for _, existing := range s.cfg.Skills.Items {
		if !strings.EqualFold(strings.TrimSpace(existing.Name), name) {
			continue
		}
		if matchedID != "" {
			return ""
		}
		matchedID = existing.ID
	}
	return matchedID
}

func (s *Store) findScheduledTaskIDForUpdateLocked(task scheduler.Task) string {
	action := strings.TrimSpace(task.Action)
	if action == "" {
		return ""
	}
	matchedID := ""
	for _, existing := range s.cfg.Agent.Schedules {
		if strings.TrimSpace(existing.Action) != action {
			continue
		}
		if matchedID != "" {
			return ""
		}
		matchedID = existing.ID
	}
	return matchedID
}

func (s *Store) findAutoSkillIDByNameLocked(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	for _, existing := range s.cfg.Skills.Items {
		if !isAutoSkillID(existing.ID) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(existing.Name), name) {
			return existing.ID
		}
	}
	return ""
}

func (s *Store) generateAutoSkillIDLocked(name, prompt string) string {
	seed := sanitizeIdentifier(name)
	if seed == "" {
		seed = sanitizeIdentifier(prompt)
	}
	if seed == "" {
		seed = "skill"
	}

	candidate := autoSkillIDPrefix + seed
	used := make(map[string]struct{}, len(s.cfg.Skills.Items))
	for _, skill := range s.cfg.Skills.Items {
		used[skill.ID] = struct{}{}
	}
	if _, exists := used[candidate]; !exists {
		return candidate
	}

	for i := 2; ; i++ {
		next := fmt.Sprintf("%s%s-%d", autoSkillIDPrefix, seed, i)
		if _, exists := used[next]; !exists {
			return next
		}
	}
}

func (s *Store) trimAutoSkillsLocked(limit int) {
	if limit <= 0 {
		s.cfg.Skills.Items = filterSkills(s.cfg.Skills.Items, func(skill Skill) bool {
			return !isAutoSkillID(skill.ID)
		})
		return
	}

	type autoSkillWithIndex struct {
		Index     int
		UpdatedAt time.Time
	}
	autoSkills := make([]autoSkillWithIndex, 0, len(s.cfg.Skills.Items))
	for i, skill := range s.cfg.Skills.Items {
		if isAutoSkillID(skill.ID) {
			autoSkills = append(autoSkills, autoSkillWithIndex{
				Index:     i,
				UpdatedAt: skill.UpdatedAt,
			})
		}
	}
	if len(autoSkills) <= limit {
		return
	}

	sort.Slice(autoSkills, func(i, j int) bool {
		if autoSkills[i].UpdatedAt.Equal(autoSkills[j].UpdatedAt) {
			return autoSkills[i].Index < autoSkills[j].Index
		}
		return autoSkills[i].UpdatedAt.Before(autoSkills[j].UpdatedAt)
	})

	removeCount := len(autoSkills) - limit
	removeIndex := make(map[int]struct{}, removeCount)
	for i := 0; i < removeCount; i++ {
		removeIndex[autoSkills[i].Index] = struct{}{}
	}

	next := make([]Skill, 0, len(s.cfg.Skills.Items)-removeCount)
	for i, skill := range s.cfg.Skills.Items {
		if _, drop := removeIndex[i]; drop {
			continue
		}
		next = append(next, skill)
	}
	s.cfg.Skills.Items = next
}
