package agent

import (
	"context"
	"fmt"
	"laughing-barnacle/internal/routine"
	"strings"
)

func (a *Agent) RunScheduledTask(ctx context.Context, action string) error {
	action = routine.NormalizeAction(strings.TrimSpace(action))

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.cfg.EnforceHumanRoutine {
		return nil
	}

	skillID, ok := routine.SkillIDFromAction(action)
	if !ok {
		err := fmt.Errorf("unknown scheduled action: %s", action)
		a.appendScheduledTaskFailureLocked(action, err)
		return err
	}

	var (
		title   string
		content string
		err     error
	)
	title, content, err = a.runGenericScheduledSkill(ctx, skillID)
	if err != nil {
		a.appendScheduledTaskFailureLocked(action, err)
		return err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if strings.TrimSpace(title) == "" {
		a.store.Append("assistant", content)
		return nil
	}
	a.store.Append("assistant", "【"+strings.TrimSpace(title)+"】\n"+content)
	return nil
}
