package agent

import (
	"laughing-barnacle/internal/conversation"
	"sort"
	"strings"
)

func selectSkillIDsForTurn(skillIndexLines []string, messages []conversation.Message) []string {
	if len(skillIndexLines) == 0 {
		return nil
	}

	focus := buildSkillFocus(messages)
	type scoredSkill struct {
		ID    string
		Score int
		Index int
	}

	seen := make(map[string]struct{}, len(skillIndexLines))
	scored := make([]scoredSkill, 0, len(skillIndexLines))
	for i, raw := range skillIndexLines {
		skillID, line := parseSkillIndexLine(raw)
		if skillID == "" || line == "" {
			continue
		}
		if _, exists := seen[skillID]; exists {
			continue
		}
		seen[skillID] = struct{}{}
		scored = append(scored, scoredSkill{
			ID:    skillID,
			Score: scoreSkillPrompt(line, focus),
			Index: i,
		})
	}
	if len(scored) == 0 {
		return nil
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Index < scored[j].Index
	})

	selected := make([]string, 0, min(maxInjectedSkillPrompts, len(scored)))
	for _, item := range scored {
		if len(selected) >= maxInjectedSkillPrompts {
			break
		}
		if item.Score < minInjectedSkillScore {
			break
		}
		selected = append(selected, item.ID)
	}
	return selected
}

func parseSkillIndexLine(raw string) (skillID string, line string) {
	fields := parseSkillIndexFields(raw)
	skillID = fields["skill_id"]
	if skillID == "" {
		return "", ""
	}
	line = compactSkillIndexLine(fields)
	return skillID, line
}

func compactSkillIndexByIDs(rawLines []string, selectedIDs []string) []string {
	if len(rawLines) == 0 {
		return nil
	}

	selectedSet := make(map[string]struct{}, len(selectedIDs))
	for _, id := range selectedIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			selectedSet[trimmed] = struct{}{}
		}
	}

	useAll := len(selectedSet) == 0
	seen := make(map[string]struct{}, len(rawLines))
	out := make([]string, 0, len(rawLines))
	for _, raw := range rawLines {
		fields := parseSkillIndexFields(raw)
		skillID := fields["skill_id"]
		if skillID == "" {
			continue
		}
		if _, exists := seen[skillID]; exists {
			continue
		}
		seen[skillID] = struct{}{}
		if !useAll {
			if _, ok := selectedSet[skillID]; !ok {
				continue
			}
		}

		line := compactSkillIndexLine(fields)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func parseSkillIndexFields(raw string) map[string]string {
	text := trimRunes(strings.TrimSpace(raw), maxSingleSkillPromptRunes)
	if text == "" {
		return nil
	}
	fields := make(map[string]string, 6)
	for _, part := range strings.Split(text, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		value := strings.TrimSpace(kv[1])
		if key == "" || value == "" {
			continue
		}
		fields[key] = value
	}
	return fields
}

func compactSkillIndexLine(fields map[string]string) string {
	if len(fields) == 0 {
		return ""
	}
	skillID := strings.TrimSpace(fields["skill_id"])
	if skillID == "" {
		return ""
	}
	name := trimRunes(strings.TrimSpace(fields["name"]), 20)
	description := trimRunes(strings.TrimSpace(fields["description"]), 24)

	parts := []string{"skill_id=" + skillID}
	if name != "" {
		parts = append(parts, "name="+name)
	}
	if description != "" {
		parts = append(parts, "description="+description)
	}
	return trimRunes(strings.Join(parts, " | "), maxSingleSkillPromptRunes)
}

func buildSkillFocus(messages []conversation.Message) string {
	if len(messages) == 0 {
		return ""
	}

	// Only use recent user turns to avoid stale summary/assistant text
	// continuously pulling irrelevant operational skills into each request.
	userFocus := make([]string, 0, maxSkillFocusUserMessages)
	for i := len(messages) - 1; i >= 0 && len(userFocus) < maxSkillFocusUserMessages; i-- {
		msg := messages[i]
		if msg.Role != "user" {
			continue
		}
		if v := strings.TrimSpace(msg.Content); v != "" {
			userFocus = append(userFocus, v)
		}
	}
	if len(userFocus) == 0 {
		return ""
	}

	var b strings.Builder
	for i := len(userFocus) - 1; i >= 0; i-- {
		b.WriteString(userFocus[i])
		b.WriteString("\n")
	}
	return strings.ToLower(b.String())
}

func scoreSkillPrompt(prompt, focus string) int {
	if strings.TrimSpace(prompt) == "" {
		return 0
	}
	if strings.TrimSpace(focus) == "" {
		return 1
	}

	score := 1
	tokens := skillTokenPattern.FindAllString(strings.ToLower(prompt), -1)
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		if strings.Contains(focus, token) {
			runes := len([]rune(token))
			switch {
			case runes >= 6:
				score += 3
			case runes >= 3:
				score += 2
			default:
				score++
			}
		}
	}
	if strings.Contains(prompt, "必须") || strings.Contains(prompt, "默认") || strings.Contains(prompt, "优先") {
		score++
	}
	return score
}
