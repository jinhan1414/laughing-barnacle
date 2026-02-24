package mcp

import (
	"fmt"
	"strings"
	"time"

	"laughing-barnacle/internal/scheduler"
)

func (s *Store) ListScheduledTasks() []scheduler.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneScheduledTasks(s.cfg.Agent.Schedules)
}

func (s *Store) UpsertScheduledTask(task scheduler.Task) error {
	task.ID = strings.TrimSpace(task.ID)
	task.Name = strings.TrimSpace(task.Name)
	task.Description = strings.TrimSpace(task.Description)
	task.Action = strings.TrimSpace(task.Action)
	task.CronExpr = strings.TrimSpace(task.CronExpr)
	task.LastStatus = strings.TrimSpace(task.LastStatus)
	task.LastMessage = trimSkillText(task.LastMessage, maxTaskRunMessageRunes)

	s.mu.Lock()
	defer s.mu.Unlock()

	if task.ID == "" {
		task.ID = s.findScheduledTaskIDForUpdateLocked(task)
	}
	if task.ID == "" {
		task.ID = generateUniqueTaskID(s.cfg.Agent.Schedules, task.Name, task.Action)
	}
	if task.Name == "" {
		task.Name = task.ID
	}
	if task.Description == "" {
		task.Description = trimSkillText(task.Name, 120)
	}
	if err := validateScheduledTask(task); err != nil {
		return err
	}

	now := time.Now()
	task.UpdatedAt = now

	updated := false
	for i := range s.cfg.Agent.Schedules {
		if s.cfg.Agent.Schedules[i].ID == task.ID {
			task.LastRunAt = s.cfg.Agent.Schedules[i].LastRunAt
			task.LastStatus = strings.TrimSpace(s.cfg.Agent.Schedules[i].LastStatus)
			task.LastMessage = strings.TrimSpace(s.cfg.Agent.Schedules[i].LastMessage)
			s.cfg.Agent.Schedules[i] = task
			updated = true
			break
		}
	}
	if !updated {
		s.cfg.Agent.Schedules = append(s.cfg.Agent.Schedules, task)
	}

	s.cfg.Agent.Schedules = normalizeScheduledTasks(s.cfg.Agent.Schedules)
	return s.persistLocked()
}

func (s *Store) DeleteScheduledTask(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("scheduled task id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := make([]scheduler.Task, 0, len(s.cfg.Agent.Schedules))
	for _, task := range s.cfg.Agent.Schedules {
		if task.ID != id {
			next = append(next, task)
		}
	}
	s.cfg.Agent.Schedules = next
	return s.persistLocked()
}

func (s *Store) SetScheduledTaskEnabled(id string, enabled bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("scheduled task id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.cfg.Agent.Schedules {
		if s.cfg.Agent.Schedules[i].ID != id {
			continue
		}
		s.cfg.Agent.Schedules[i].Enabled = enabled
		s.cfg.Agent.Schedules[i].UpdatedAt = time.Now()
		s.cfg.Agent.Schedules = normalizeScheduledTasks(s.cfg.Agent.Schedules)
		return s.persistLocked()
	}
	return fmt.Errorf("scheduled task %q not found", id)
}

func (s *Store) MarkScheduledTaskRun(id string, runAt time.Time, status string, message string) error {
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	message = trimSkillText(message, maxTaskRunMessageRunes)
	if id == "" {
		return fmt.Errorf("scheduled task id is required")
	}
	if runAt.IsZero() {
		return fmt.Errorf("scheduled task run time is required")
	}
	if status == "" {
		status = "success"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.cfg.Agent.Schedules {
		if s.cfg.Agent.Schedules[i].ID != id {
			continue
		}
		s.cfg.Agent.Schedules[i].LastRunAt = runAt
		s.cfg.Agent.Schedules[i].LastStatus = status
		s.cfg.Agent.Schedules[i].LastMessage = message
		s.cfg.Agent.Schedules[i].UpdatedAt = time.Now()
		s.cfg.Agent.Schedules = normalizeScheduledTasks(s.cfg.Agent.Schedules)
		return s.persistLocked()
	}
	return fmt.Errorf("scheduled task %q not found", id)
}
