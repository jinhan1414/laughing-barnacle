package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeStore struct {
	mu    sync.Mutex
	tasks []Task
	marks []mark
}

type mark struct {
	id      string
	runAt   time.Time
	status  string
	message string
}

func (f *fakeStore) ListScheduledTasks() []Task {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Task, len(f.tasks))
	copy(out, f.tasks)
	return out
}

func (f *fakeStore) MarkScheduledTaskRun(id string, runAt time.Time, status string, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marks = append(f.marks, mark{id: id, runAt: runAt, status: status, message: message})
	for i := range f.tasks {
		if f.tasks[i].ID == id {
			f.tasks[i].LastRunAt = runAt
			f.tasks[i].LastStatus = status
			f.tasks[i].LastMessage = message
		}
	}
	return nil
}

type fakeRunner struct {
	mu      sync.Mutex
	actions []string
	err     error
	delay   time.Duration

	current int32
	maxSeen int32
}

func (f *fakeRunner) RunScheduledTask(_ context.Context, action string) error {
	now := atomic.AddInt32(&f.current, 1)
	for {
		max := atomic.LoadInt32(&f.maxSeen)
		if now <= max {
			break
		}
		if atomic.CompareAndSwapInt32(&f.maxSeen, max, now) {
			break
		}
	}
	defer atomic.AddInt32(&f.current, -1)

	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	f.actions = append(f.actions, action)
	f.mu.Unlock()
	return f.err
}

func TestEngineStartReloadAndStop(t *testing.T) {
	store := &fakeStore{tasks: []Task{{
		ID:       "morning-plan",
		Action:   "skill:morning-planning",
		CronExpr: "* * * * *",
		Enabled:  true,
	}}}
	runner := &fakeRunner{}
	engine := NewEngine(store, runner, nil)

	if err := engine.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if !engine.started {
		t.Fatalf("expected engine started")
	}
	if len(engine.entries) != 1 {
		t.Fatalf("expected one scheduled entry, got %d", len(engine.entries))
	}

	store.mu.Lock()
	store.tasks = append(store.tasks, Task{
		ID:       "night-review",
		Action:   "skill:night-reflection-evolution",
		CronExpr: "30 0 * * *",
		Enabled:  true,
	})
	store.mu.Unlock()
	if err := engine.Reload(); err != nil {
		t.Fatalf("Reload error: %v", err)
	}
	if len(engine.entries) != 2 {
		t.Fatalf("expected two scheduled entries after reload, got %d", len(engine.entries))
	}

	engine.Stop()
	if engine.started {
		t.Fatalf("expected engine stopped")
	}
}

func TestEngineRunNow_MarksSuccess(t *testing.T) {
	store := &fakeStore{tasks: []Task{{
		ID:       "morning-plan",
		Action:   "skill:morning-planning",
		CronExpr: "* * * * *",
		Enabled:  true,
	}}}
	runner := &fakeRunner{}
	engine := NewEngine(store, runner, nil)

	if err := engine.RunNow("morning-plan"); err != nil {
		t.Fatalf("RunNow error: %v", err)
	}

	runner.mu.Lock()
	actions := append([]string(nil), runner.actions...)
	runner.mu.Unlock()
	if len(actions) != 1 || actions[0] != "skill:morning-planning" {
		t.Fatalf("unexpected run actions: %+v", actions)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.marks) != 1 {
		t.Fatalf("expected 1 mark, got %d", len(store.marks))
	}
	if store.marks[0].status != "success" {
		t.Fatalf("expected success mark, got %+v", store.marks[0])
	}
	if store.marks[0].message != "manual_run" {
		t.Fatalf("expected manual_run message, got %+v", store.marks[0])
	}
}

func TestEngineRunNow_MarksError(t *testing.T) {
	store := &fakeStore{tasks: []Task{{
		ID:       "night-review",
		Action:   "skill:night-reflection-evolution",
		CronExpr: "* * * * *",
		Enabled:  true,
	}}}
	runner := &fakeRunner{err: fmt.Errorf("runner failed")}
	engine := NewEngine(store, runner, nil)

	err := engine.RunNow("night-review")
	if err == nil {
		t.Fatalf("expected run error")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.marks) != 1 {
		t.Fatalf("expected 1 mark, got %d", len(store.marks))
	}
	if store.marks[0].status != "error" {
		t.Fatalf("expected error mark, got %+v", store.marks[0])
	}
	if store.marks[0].message == "" {
		t.Fatalf("expected non-empty error message")
	}
}
