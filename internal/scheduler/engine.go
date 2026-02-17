package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

type Task struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Action      string    `json:"action"`
	CronExpr    string    `json:"cron_expr"`
	Enabled     bool      `json:"enabled"`
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
	LastStatus  string    `json:"last_status,omitempty"`
	LastMessage string    `json:"last_message,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type TaskStore interface {
	ListScheduledTasks() []Task
	MarkScheduledTaskRun(id string, runAt time.Time, status string, message string) error
}

type TaskRunner interface {
	RunScheduledTask(ctx context.Context, action string) error
}

type Logger interface {
	Printf(format string, v ...any)
}

type Engine struct {
	store  TaskStore
	runner TaskRunner
	logger Logger
	parser cron.Parser

	mu      sync.Mutex
	cron    *cron.Cron
	entries map[string]cron.EntryID
	running map[string]struct{}
	started bool

	execMu sync.Mutex
}

var ErrTaskAlreadyRunning = errors.New("scheduled task is already running")

func NewEngine(store TaskStore, runner TaskRunner, logger Logger) *Engine {
	return &Engine{
		store:  store,
		runner: runner,
		logger: logger,
		parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

func (e *Engine) Start() error {
	if e == nil || e.store == nil || e.runner == nil {
		return errors.New("scheduler engine not initialized")
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started {
		return nil
	}

	e.cron = cron.New(
		cron.WithParser(e.parser),
		cron.WithLocation(time.Local),
	)
	e.entries = make(map[string]cron.EntryID)
	e.running = make(map[string]struct{})
	if err := e.reloadLocked(); err != nil {
		return err
	}
	e.cron.Start()
	e.started = true
	return nil
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.started || e.cron == nil {
		return
	}
	ctx := e.cron.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
	}
	e.cron = nil
	e.entries = nil
	e.running = nil
	e.started = false
}

func (e *Engine) Reload() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.started || e.cron == nil {
		return nil
	}
	return e.reloadLocked()
}

func (e *Engine) reloadLocked() error {
	for taskID, entryID := range e.entries {
		e.cron.Remove(entryID)
		delete(e.entries, taskID)
	}

	tasks := e.store.ListScheduledTasks()
	for _, task := range tasks {
		if !task.Enabled {
			continue
		}
		task.ID = strings.TrimSpace(task.ID)
		task.Action = strings.TrimSpace(task.Action)
		task.CronExpr = strings.TrimSpace(task.CronExpr)
		if task.ID == "" || task.Action == "" || task.CronExpr == "" {
			continue
		}

		taskCopy := task
		entryID, err := e.cron.AddFunc(taskCopy.CronExpr, func() {
			_, _ = e.executeTask(taskCopy, "cron")
		})
		if err != nil {
			e.logf("schedule %s parse error: %v", taskCopy.ID, err)
			continue
		}
		e.entries[taskCopy.ID] = entryID
	}
	return nil
}

func (e *Engine) RunNow(taskID string) error {
	if e == nil || e.store == nil || e.runner == nil {
		return errors.New("scheduler engine not initialized")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task id is required")
	}
	task, ok := findTaskByID(e.store.ListScheduledTasks(), taskID)
	if !ok {
		return fmt.Errorf("scheduled task %q not found", taskID)
	}
	_, err := e.executeTask(task, "manual")
	return err
}

func (e *Engine) HasRunningTask() bool {
	if e == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.running) > 0
}

func (e *Engine) executeTask(task Task, source string) (time.Time, error) {
	task.ID = strings.TrimSpace(task.ID)
	task.Action = strings.TrimSpace(task.Action)
	if task.ID == "" || task.Action == "" {
		return time.Time{}, fmt.Errorf("invalid task payload")
	}

	runAt := time.Now().Truncate(time.Second)
	if !e.beginTask(task.ID) {
		_ = e.store.MarkScheduledTaskRun(task.ID, runAt, "skipped", "already_running")
		return runAt, ErrTaskAlreadyRunning
	}
	defer e.endTask(task.ID)

	// Serialize executions to avoid cross-task races when multiple jobs share one fire time.
	e.execMu.Lock()
	defer e.execMu.Unlock()

	if err := e.runner.RunScheduledTask(context.Background(), task.Action); err != nil {
		e.logf("schedule %s (%s) run error: %v", task.ID, task.Action, err)
		if markErr := e.store.MarkScheduledTaskRun(task.ID, runAt, "error", err.Error()); markErr != nil {
			e.logf("schedule %s mark error state failed: %v", task.ID, markErr)
		}
		return runAt, err
	}
	message := ""
	if strings.TrimSpace(source) == "manual" {
		message = "manual_run"
	}
	if err := e.store.MarkScheduledTaskRun(task.ID, runAt, "success", message); err != nil {
		e.logf("schedule %s mark success state failed: %v", task.ID, err)
	}
	return runAt, nil
}

func (e *Engine) beginTask(taskID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running == nil {
		e.running = make(map[string]struct{})
	}
	if _, exists := e.running[taskID]; exists {
		return false
	}
	e.running[taskID] = struct{}{}
	return true
}

func (e *Engine) endTask(taskID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running == nil {
		return
	}
	delete(e.running, taskID)
}

func findTaskByID(tasks []Task, taskID string) (Task, bool) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Task{}, false
	}
	for _, task := range tasks {
		if strings.TrimSpace(task.ID) == taskID {
			return task, true
		}
	}
	return Task{}, false
}

func (e *Engine) logf(format string, v ...any) {
	if e.logger != nil {
		e.logger.Printf(format, v...)
	}
}
