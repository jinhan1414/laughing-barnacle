package agent

import (
	"context"
	"fmt"
	"strings"
)

func (a *Agent) runGenericScheduledSkill(ctx context.Context, skillID string) (title string, content string, err error) {
	skillName, skillPrompt, ok := a.readScheduledSkill(skillID)
	if !ok {
		return "", "", fmt.Errorf("scheduled skill %q not found or not enabled", skillID)
	}
	summary, messages := a.store.Snapshot()
	content, err = a.generateGenericScheduledOutput(ctx, skillID, skillPrompt, summary, messages)
	if err != nil {
		return "", "", err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return "", "", nil
	}
	if strings.TrimSpace(skillName) == "" {
		skillName = skillID
	}
	return "定时任务（自动）" + skillName, content, nil
}
