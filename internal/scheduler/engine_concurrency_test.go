package scheduler

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestEngineRunNow_SameTaskConcurrentSkipsSecond(t *testing.T) {
	store := &fakeStore{tasks: []Task{{
		ID:       "morning-plan",
		Action:   "morning_planning",
		CronExpr: "* * * * *",
		Enabled:  true,
	}}}
	runner := &fakeRunner{delay: 120 * time.Millisecond}
	engine := NewEngine(store, runner, nil)

	start := make(chan struct{})
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			errCh <- engine.RunNow("morning-plan")
		}()
	}
	close(start)
	err1 := <-errCh
	err2 := <-errCh

	var okCount, skipCount int
	for _, err := range []error{err1, err2} {
		switch {
		case err == nil:
			okCount++
		case errors.Is(err, ErrTaskAlreadyRunning):
			skipCount++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if okCount != 1 || skipCount != 1 {
		t.Fatalf("expected one success and one skipped, got success=%d skipped=%d", okCount, skipCount)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.marks) != 2 {
		t.Fatalf("expected 2 marks, got %d", len(store.marks))
	}
	seenSuccess := false
	seenSkipped := false
	for _, m := range store.marks {
		if m.status == "success" {
			seenSuccess = true
		}
		if m.status == "skipped" && m.message == "already_running" {
			seenSkipped = true
		}
	}
	if !seenSuccess || !seenSkipped {
		t.Fatalf("expected success and skipped marks, got %+v", store.marks)
	}
}

func TestEngineRunNow_DifferentTasksSameTimeExecuteSerially(t *testing.T) {
	store := &fakeStore{tasks: []Task{
		{ID: "morning-plan", Action: "morning_planning", CronExpr: "* * * * *", Enabled: true},
		{ID: "night-review", Action: "night_reflection_evolution", CronExpr: "* * * * *", Enabled: true},
	}}
	runner := &fakeRunner{delay: 80 * time.Millisecond}
	engine := NewEngine(store, runner, nil)

	start := make(chan struct{})
	errCh := make(chan error, 2)
	go func() {
		<-start
		errCh <- engine.RunNow("morning-plan")
	}()
	go func() {
		<-start
		errCh <- engine.RunNow("night-review")
	}()
	close(start)

	if err := <-errCh; err != nil {
		t.Fatalf("unexpected first run error: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected second run error: %v", err)
	}

	if got := atomic.LoadInt32(&runner.maxSeen); got > 1 {
		t.Fatalf("expected serial execution (max concurrency 1), got %d", got)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.actions) != 2 {
		t.Fatalf("expected 2 actions, got %+v", runner.actions)
	}
}

func TestEngineReload_SkipsInvalidExpr(t *testing.T) {
	store := &fakeStore{tasks: []Task{{
		ID:       "bad-task",
		Action:   "morning_planning",
		CronExpr: "invalid",
		Enabled:  true,
	}}}
	runner := &fakeRunner{}
	engine := NewEngine(store, runner, nil)

	if err := engine.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer engine.Stop()

	if len(engine.entries) != 0 {
		t.Fatalf("expected no scheduled entry for invalid expr, got %d", len(engine.entries))
	}
}

func TestEngineHasRunningTask(t *testing.T) {
	store := &fakeStore{tasks: []Task{{
		ID:       "morning-plan",
		Action:   "morning_planning",
		CronExpr: "* * * * *",
		Enabled:  true,
	}}}
	runner := &fakeRunner{delay: 120 * time.Millisecond}
	engine := NewEngine(store, runner, nil)

	if engine.HasRunningTask() {
		t.Fatalf("expected no running task initially")
	}

	done := make(chan error, 1)
	go func() {
		done <- engine.RunNow("morning-plan")
	}()

	deadline := time.After(2 * time.Second)
	for !engine.HasRunningTask() {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("RunNow error before observing running state: %v", err)
			}
			t.Fatalf("run completed before observing running state")
		case <-deadline:
			t.Fatalf("timeout waiting for running task state")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("RunNow error: %v", err)
	}
	if engine.HasRunningTask() {
		t.Fatalf("expected no running task after completion")
	}
}
