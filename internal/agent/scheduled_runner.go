package agent

import (
	"context"
	"fmt"
	"laughing-barnacle/internal/routine"
	"strings"
)

func (a *Agent) RunScheduledHumanRoutine(ctx context.Context) error {
	now := a.nowFn()
	if isSleepWindow(now) {
		return a.RunScheduledTask(ctx, routine.ActionNightReflectionEvolution)
	}
	return a.RunScheduledTask(ctx, routine.ActionMorningPlanning)
}

func (a *Agent) RunScheduledTask(ctx context.Context, action string) error {
	action = routine.NormalizeAction(strings.TrimSpace(action))

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.cfg.EnforceHumanRoutine {
		return nil
	}

	now := a.nowFn()
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
	switch {
	case routine.IsNightReflectionSkillID(skillID):
		title = "夜间复盘（自动）"
		content, err = a.runNightReflectionAndEvolution(ctx, now, skillID)
	case routine.IsMorningPlanningSkillID(skillID):
		title = "晨间规划（自动）"
		content, err = a.runMorningPlanning(ctx, now, skillID)
	default:
		title, content, err = a.runGenericScheduledSkill(ctx, now, skillID)
	}
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
