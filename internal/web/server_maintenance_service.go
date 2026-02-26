package web

import (
	"context"
	"fmt"
	"strings"
	"time"

	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/routine"
	"laughing-barnacle/internal/scheduler"
	"laughing-barnacle/internal/skills"
)

type scheduleDeleteResult struct {
	Message string
	Warning string
}

func (s *Server) saveMCPService(service mcp.Service) error {
	if s.mcpStore == nil {
		return fmt.Errorf("mcp store unavailable")
	}
	if err := s.mcpStore.UpsertService(service); err != nil {
		return err
	}
	if s.mcpTools != nil {
		s.mcpTools.InvalidateCache()
	}
	return nil
}

func (s *Server) deleteMCPService(id string) error {
	if s.mcpStore == nil {
		return fmt.Errorf("mcp store unavailable")
	}
	if err := s.mcpStore.DeleteService(strings.TrimSpace(id)); err != nil {
		return err
	}
	if s.mcpTools != nil {
		s.mcpTools.InvalidateCache()
	}
	return nil
}

func (s *Server) toggleMCPService(id string, enabled bool) error {
	if s.mcpStore == nil {
		return fmt.Errorf("mcp store unavailable")
	}
	if err := s.mcpStore.SetEnabled(strings.TrimSpace(id), enabled); err != nil {
		return err
	}
	if s.mcpTools != nil {
		s.mcpTools.InvalidateCache()
	}
	return nil
}

func (s *Server) installSkill(ctx context.Context, rawURL string) (skills.Skill, error) {
	if s.skillStore == nil {
		return skills.Skill{}, fmt.Errorf("skill store unavailable")
	}
	return s.skillStore.InstallFromSkillsSH(ctx, strings.TrimSpace(rawURL))
}

func (s *Server) saveSkill(skill skills.Skill) error {
	if s.skillStore == nil {
		return fmt.Errorf("skill store unavailable")
	}
	return s.skillStore.UpsertSkill(skill)
}

func (s *Server) deleteSkill(id string) error {
	if s.skillStore == nil {
		return fmt.Errorf("skill store unavailable")
	}
	return s.skillStore.DeleteSkill(strings.TrimSpace(id))
}

func (s *Server) toggleSkill(id string, enabled bool) error {
	if s.skillStore == nil {
		return fmt.Errorf("skill store unavailable")
	}
	return s.skillStore.SetSkillEnabled(strings.TrimSpace(id), enabled)
}

func (s *Server) saveSchedule(task scheduler.Task) error {
	if s.mcpStore == nil {
		return fmt.Errorf("mcp store unavailable")
	}
	if err := s.validateScheduleActionSkill(task.Action); err != nil {
		return err
	}
	if err := s.mcpStore.UpsertScheduledTask(task); err != nil {
		return err
	}
	return s.reloadScheduler()
}

func (s *Server) deleteSchedule(id string) (scheduleDeleteResult, error) {
	if s.mcpStore == nil {
		return scheduleDeleteResult{}, fmt.Errorf("mcp store unavailable")
	}
	taskID := strings.TrimSpace(id)
	tasks := s.mcpStore.ListScheduledTasks()
	task, found := findScheduledTaskByID(tasks, taskID)
	if err := s.mcpStore.DeleteScheduledTask(taskID); err != nil {
		return scheduleDeleteResult{}, err
	}
	if err := s.reloadScheduler(); err != nil {
		return scheduleDeleteResult{}, err
	}
	return s.appendScheduleDeleteSkillResult(tasks, task, found)
}

func (s *Server) appendScheduleDeleteSkillResult(tasks []scheduler.Task, task scheduler.Task, found bool) (scheduleDeleteResult, error) {
	result := scheduleDeleteResult{Message: fmt.Sprintf("定时任务 %s 已删除", strings.TrimSpace(task.ID))}
	if strings.TrimSpace(task.ID) == "" {
		result.Message = "定时任务已删除"
	}
	if s.skillStore == nil || !found {
		return result, nil
	}
	skillID, ok := routine.SkillIDFromAction(task.Action)
	if !ok {
		return result, nil
	}
	if s.isReferencedByOtherTask(tasks, strings.TrimSpace(task.ID), skillID) {
		result.Message += fmt.Sprintf("；Skill %s 仍被其他任务引用，未删除", skillID)
		return result, nil
	}
	if s.isBuiltinSkill(skillID) {
		result.Message += fmt.Sprintf("；Skill %s 为内置技能，保留", skillID)
		return result, nil
	}
	if err := s.skillStore.DeleteSkill(skillID); err != nil {
		result.Warning = fmt.Sprintf("删除关联 Skill %s 失败: %v", skillID, err)
		return result, nil
	}
	result.Message += fmt.Sprintf("；关联 Skill %s 已删除", skillID)
	return result, nil
}

func (s *Server) isReferencedByOtherTask(tasks []scheduler.Task, deletedTaskID, skillID string) bool {
	for _, task := range tasks {
		if strings.TrimSpace(task.ID) == deletedTaskID {
			continue
		}
		otherSkillID, ok := routine.SkillIDFromAction(task.Action)
		if ok && strings.TrimSpace(otherSkillID) == strings.TrimSpace(skillID) {
			return true
		}
	}
	return false
}

func (s *Server) toggleSchedule(id string, enabled bool) error {
	if s.mcpStore == nil {
		return fmt.Errorf("mcp store unavailable")
	}
	if err := s.mcpStore.SetScheduledTaskEnabled(strings.TrimSpace(id), enabled); err != nil {
		return err
	}
	return s.reloadScheduler()
}

func (s *Server) runScheduleNow(ctx context.Context, taskID string) error {
	if s.mcpStore == nil {
		return fmt.Errorf("mcp store unavailable")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("任务 id 不能为空")
	}
	task, ok := findScheduledTaskByID(s.mcpStore.ListScheduledTasks(), taskID)
	if !ok {
		return fmt.Errorf("定时任务 %s 不存在", taskID)
	}
	if err := s.validateScheduleActionSkill(task.Action); err != nil {
		s.markScheduleRunError(task.ID, err.Error())
		return fmt.Errorf("立即执行失败：%s", err.Error())
	}
	if s.scheduler != nil {
		if err := s.scheduler.RunNow(taskID); err != nil {
			return fmt.Errorf("立即执行失败：%s", err.Error())
		}
		return nil
	}
	return s.runScheduleNowByAgent(ctx, task)
}

func (s *Server) runScheduleNowByAgent(ctx context.Context, task scheduler.Task) error {
	if s.agent == nil {
		return fmt.Errorf("agent 未初始化")
	}
	runAt := time.Now().Truncate(time.Second)
	if err := s.agent.RunScheduledTask(ctx, task.Action); err != nil {
		s.markScheduleRunError(task.ID, err.Error())
		return fmt.Errorf("立即执行失败：%s", err.Error())
	}
	if err := s.mcpStore.MarkScheduledTaskRun(task.ID, runAt, "success", "manual_run"); err != nil {
		return fmt.Errorf("状态写回失败：%s", err.Error())
	}
	return nil
}

func (s *Server) markScheduleRunError(taskID, message string) {
	if s.mcpStore == nil {
		return
	}
	_ = s.mcpStore.MarkScheduledTaskRun(strings.TrimSpace(taskID), time.Now().Truncate(time.Second), "error", strings.TrimSpace(message))
}

func (s *Server) reloadScheduler() error {
	if s.scheduler == nil {
		return nil
	}
	if err := s.scheduler.Reload(); err != nil {
		return fmt.Errorf("调度器重载失败：%s", err.Error())
	}
	return nil
}
