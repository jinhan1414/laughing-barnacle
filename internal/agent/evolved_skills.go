package agent

import (
	"strings"
)

func (a *Agent) applyNightEvolvedSkills(skills []evolvedSkill) int {
	if len(skills) == 0 || a.skills == nil {
		return 0
	}
	writer, ok := a.skills.(AutoSkillWriter)
	if !ok {
		return 0
	}

	updated := 0
	for _, skill := range skills {
		if strings.TrimSpace(skill.Name) == "" || strings.TrimSpace(skill.Prompt) == "" {
			continue
		}
		if err := writer.UpsertAutoSkill(skill.Name, skill.Prompt); err == nil {
			updated++
		}
	}
	return updated
}

func normalizeEvolvedSkills(raw []struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}) []evolvedSkill {
	if len(raw) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]evolvedSkill, 0, len(raw))
	for _, item := range raw {
		name := trimRunes(strings.TrimSpace(item.Name), maxEvolvedSkillNameRunes)
		prompt := trimRunes(strings.TrimSpace(item.Prompt), maxEvolvedSkillPromptRunes)
		if name == "" || prompt == "" {
			continue
		}
		key := strings.ToLower(name) + "\n" + strings.ToLower(prompt)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, evolvedSkill{
			Name:   name,
			Prompt: prompt,
		})
		if len(out) >= maxNightEvolvedSkills {
			break
		}
	}
	return out
}
