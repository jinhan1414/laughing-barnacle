package mcp

import (
	"sort"
	"strconv"
	"strings"

	"laughing-barnacle/internal/routine"
	"laughing-barnacle/internal/scheduler"
)

func filterSkills(skills []Skill, keep func(Skill) bool) []Skill {
	if len(skills) == 0 {
		return nil
	}
	next := make([]Skill, 0, len(skills))
	for _, skill := range skills {
		if keep(skill) {
			next = append(next, skill)
		}
	}
	return next
}

func isAutoSkillID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), autoSkillIDPrefix)
}

func normalizeAndMergeScheduledTasks(tasks []scheduler.Task) ([]scheduler.Task, bool, error) {
	merged := normalizeScheduledTasks(tasks)
	changed := !scheduledTasksEqual(merged, tasks)
	final := make([]scheduler.Task, 0, len(merged))
	seen := map[string]struct{}{}
	for _, task := range merged {
		if _, ok := seen[task.ID]; ok {
			continue
		}
		if err := validateScheduledTask(task); err != nil {
			return nil, false, err
		}
		final = append(final, task)
		seen[task.ID] = struct{}{}
	}
	final = normalizeScheduledTasks(final)
	return final, changed || !scheduledTasksEqual(final, tasks), nil
}

func normalizeScheduledTasks(tasks []scheduler.Task) []scheduler.Task {
	if len(tasks) == 0 {
		return nil
	}

	out := make([]scheduler.Task, 0, len(tasks))
	for _, task := range tasks {
		task.ID = strings.TrimSpace(task.ID)
		task.Name = strings.TrimSpace(task.Name)
		task.Description = trimSkillText(strings.TrimSpace(strings.ReplaceAll(task.Description, "\n", " ")), 160)
		task.Action = routine.NormalizeAction(strings.TrimSpace(task.Action))
		task.CronExpr = strings.TrimSpace(task.CronExpr)
		task.LastStatus = strings.TrimSpace(task.LastStatus)
		task.LastMessage = trimSkillText(task.LastMessage, maxTaskRunMessageRunes)
		if task.ID == "" {
			continue
		}
		if task.Name == "" {
			task.Name = task.ID
		}
		if task.Description == "" {
			task.Description = trimSkillText(task.Name, 160)
		}
		out = append(out, task)
	}
	if len(out) == 0 {
		return nil
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func cloneScheduledTasks(in []scheduler.Task) []scheduler.Task {
	if len(in) == 0 {
		return nil
	}
	out := make([]scheduler.Task, len(in))
	copy(out, in)
	return out
}

func scheduledTasksEqual(a, b []scheduler.Task) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !scheduledTaskEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func scheduledTaskEqual(a, b scheduler.Task) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.Name) == strings.TrimSpace(b.Name) &&
		strings.TrimSpace(a.Description) == strings.TrimSpace(b.Description) &&
		strings.TrimSpace(a.Action) == strings.TrimSpace(b.Action) &&
		strings.TrimSpace(a.CronExpr) == strings.TrimSpace(b.CronExpr) &&
		a.Enabled == b.Enabled &&
		a.LastRunAt.Equal(b.LastRunAt) &&
		strings.TrimSpace(a.LastStatus) == strings.TrimSpace(b.LastStatus) &&
		strings.TrimSpace(a.LastMessage) == strings.TrimSpace(b.LastMessage) &&
		a.UpdatedAt.Equal(b.UpdatedAt)
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

func normalizeSkillStandardName(skill Skill) string {
	raw := strings.TrimSpace(skill.ID)
	if raw == "" {
		raw = strings.TrimSpace(skill.Name)
	}
	raw = strings.ToLower(raw)

	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
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

	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "skill"
	}
	if len(name) > 64 {
		name = strings.Trim(name[:64], "-")
		if name == "" {
			name = "skill"
		}
	}
	return name
}

func renderSkillMarkdown(skill Skill) string {
	prompt := strings.TrimSpace(skill.Prompt)
	if prompt == "" {
		return ""
	}
	manifestName := normalizeSkillStandardName(skill)
	description := normalizeSkillDescription(skill.Description, skill.Name, skill.Prompt)
	if description == "" {
		return ""
	}

	return strings.TrimSpace(
		"---\n" +
			"name: " + quoteYAMLString(manifestName) + "\n" +
			"description: " + quoteYAMLString(description) + "\n" +
			"---\n\n" +
			prompt,
	)
}

func quoteYAMLString(v string) string {
	return strconv.Quote(strings.TrimSpace(strings.ReplaceAll(v, "\n", " ")))
}
