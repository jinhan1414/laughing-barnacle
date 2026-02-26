package agent

import (
	"context"
	"encoding/json"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"strconv"
	"strings"
)

func (a *Agent) generateGenericScheduledOutput(ctx context.Context, skillID, skillPrompt, summary string, messages []conversation.Message) (string, error) {
	currentSystemPrompt, currentCompressionPrompt := a.resolvePromptsLocked()
	resp, err := a.llm.Chat(ctx, llm.ChatRequest{
		Purpose: scheduledSkillPurpose(skillID),
		Model:   a.cfg.Model,
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: "你是数字分身定时任务执行器。必须严格遵循技能说明，若技能要求 JSON 则仅输出 JSON。",
			},
			{
				Role: "user",
				Content: strings.TrimSpace(
					"技能说明（来自 SKILL.md 正文）：\n" + strings.TrimSpace(skillPrompt) + "\n\n" +
						"当前系统提示词：\n" + currentSystemPrompt + "\n\n" +
						"当前压缩提示词：\n" + currentCompressionPrompt + "\n\n" +
						"历史摘要：\n" + safeOrEmpty(summary) + "\n\n" +
						"最近对话：\n" + renderConversation(lastN(messages, maxScheduledRecentMessages)) + "\n\n" +
						"请仅按技能说明完成任务并输出。",
				),
			},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}

	out := strings.TrimSpace(resp.Content)
	if out == "" {
		return "", nil
	}

	type payload struct {
		Content string `json:"content"`
		Result  string `json:"result"`
		Text    string `json:"text"`
	}
	var parsed payload
	if err := json.Unmarshal([]byte(extractJSONObject(out)), &parsed); err == nil {
		if v := strings.TrimSpace(parsed.Content); v != "" {
			return v, nil
		}
		if v := strings.TrimSpace(parsed.Result); v != "" {
			return v, nil
		}
		if v := strings.TrimSpace(parsed.Text); v != "" {
			return v, nil
		}
	}
	return out, nil
}

func (a *Agent) readScheduledSkill(skillID string) (name string, prompt string, ok bool) {
	if a.skills == nil {
		return "", "", false
	}
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return "", "", false
	}

	markdown, ok := a.skills.ReadEnabledSkillPrompt(skillID)
	if !ok {
		return "", "", false
	}
	name, prompt = parseSkillMarkdownForExecution(markdown)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", "", false
	}
	if strings.TrimSpace(name) == "" {
		name = skillID
	}
	return strings.TrimSpace(name), prompt, true
}

func parseSkillMarkdownForExecution(markdown string) (name string, prompt string) {
	text := strings.TrimSpace(strings.ReplaceAll(markdown, "\r\n", "\n"))
	if text == "" {
		return "", ""
	}
	if !strings.HasPrefix(text, "---\n") {
		return "", text
	}

	rest := strings.TrimPrefix(text, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", text
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
		if key == "name" {
			name = value
		}
	}
	if strings.TrimSpace(body) == "" {
		body = text
	}
	return strings.TrimSpace(name), strings.TrimSpace(body)
}

func scheduledSkillPurpose(skillID string) string {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return "scheduled_skill"
	}
	var b strings.Builder
	for _, r := range skillID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		if r == '-' {
			b.WriteRune('_')
			continue
		}
	}
	suffix := strings.TrimSpace(b.String())
	if suffix == "" {
		return "scheduled_skill"
	}
	return "scheduled_skill_" + suffix
}
