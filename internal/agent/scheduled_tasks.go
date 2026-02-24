package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func isSleepWindow(now time.Time) bool {
	minutes := now.Hour()*60 + now.Minute()
	sleepStartMinutes := 30
	sleepEndMinutes := 8*60 + 30
	return minutes >= sleepStartMinutes && minutes < sleepEndMinutes
}

func (a *Agent) runNightReflectionAndEvolution(ctx context.Context, now time.Time, skillID string) (string, error) {
	if a.habits == nil {
		return "", nil
	}
	today := now.Format("2006-01-02")
	if strings.TrimSpace(a.habits.GetLastSleepReviewDate()) == today {
		return "", nil
	}

	_, skillPrompt, ok := a.readScheduledSkill(skillID)
	if !ok {
		return "", fmt.Errorf("scheduled skill %q not found or not enabled", skillID)
	}

	summary, messages := a.store.Snapshot()
	reflection, systemPrompt, compressionPrompt, evolvedSkills, err := a.generateNightReflectionPayload(ctx, skillPrompt, summary, messages)
	if err != nil {
		_ = a.habits.SetLastSleepReviewDate(today)
		return "生活：已进入休息阶段并记录今日状态。\n工作：关键任务与风险已归档，明天继续推进。\n学习：延续每日学习节奏，明天聚焦一个短板。", nil
	}

	if strings.TrimSpace(systemPrompt) != "" &&
		strings.TrimSpace(compressionPrompt) != "" &&
		a.updater != nil &&
		isValidEvolvedPrompt(systemPrompt, compressionPrompt) {
		_ = a.updater.UpdateAgentPrompts(systemPrompt, compressionPrompt)
		_ = a.habits.SetLastPromptEvolutionDate(today)
	}
	evolvedCount := a.applyNightEvolvedSkills(evolvedSkills)

	_ = a.habits.SetLastSleepReviewDate(today)
	reflection = strings.TrimSpace(reflection)
	if reflection == "" {
		reflection = "生活：今日作息已收束，保持稳定节律。\n工作：今日进度已复盘，明天按优先级继续。\n学习：保持小步快跑，明天继续迭代。"
	}
	if evolvedCount > 0 {
		reflection = strings.TrimSpace(reflection + fmt.Sprintf("\n能力进化：已沉淀/更新 %d 条可复用 Skill。", evolvedCount))
	}
	return reflection, nil
}

func (a *Agent) runMorningPlanning(ctx context.Context, now time.Time, skillID string) (string, error) {
	if isSleepWindow(now) || a.habits == nil {
		return "", nil
	}
	today := now.Format("2006-01-02")
	if strings.TrimSpace(a.habits.GetLastWakePlanDate()) == today {
		return "", nil
	}

	_, skillPrompt, ok := a.readScheduledSkill(skillID)
	if !ok {
		return "", fmt.Errorf("scheduled skill %q not found or not enabled", skillID)
	}

	summary, messages := a.store.Snapshot()
	plan, err := a.generateMorningPlan(ctx, skillPrompt, summary, messages)
	if err != nil {
		_ = a.habits.SetLastWakePlanDate(today)
		return "任务回顾：请先确认昨日未完成事项并标注阻塞原因。\n今日 Top 3：1) 最关键交付 2) 次关键推进 3) 学习巩固。\n能力提升：今天复盘一个问题并沉淀为可复用方法。", nil
	}
	plan = strings.TrimSpace(plan)
	if plan == "" {
		_ = a.habits.SetLastWakePlanDate(today)
		return "任务回顾：昨日进度已记录，请先对未完成项做风险评估。\n今日 Top 3：按优先级推进核心交付、风险治理、学习巩固。\n能力提升：今天完成一次针对性复盘。", nil
	}
	_ = a.habits.SetLastWakePlanDate(today)
	return plan, nil
}

func (a *Agent) runGenericScheduledSkill(ctx context.Context, _ time.Time, skillID string) (title string, content string, err error) {
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
