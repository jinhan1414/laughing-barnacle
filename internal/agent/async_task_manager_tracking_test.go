package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type scriptedA2AProvider struct {
	mu         sync.Mutex
	sendResult A2ATaskResult
	getSteps   []a2aGetStep
	getCalls   int
}

type a2aGetStep struct {
	result A2ATaskResult
	err    error
}

func (s *scriptedA2AProvider) ListIndexLines(_ int) []string { return nil }

func (s *scriptedA2AProvider) Send(_ context.Context, _ A2ASendRequest) (A2ATaskResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendResult, nil
}

func (s *scriptedA2AProvider) GetTask(_ context.Context, _ A2ATaskQuery) (A2ATaskResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCalls++
	if len(s.getSteps) == 0 {
		return A2ATaskResult{Status: "working"}, nil
	}
	next := s.getSteps[0]
	if len(s.getSteps) > 1 {
		s.getSteps = s.getSteps[1:]
	}
	if next.err != nil {
		return A2ATaskResult{}, next.err
	}
	return next.result, nil
}

func (s *scriptedA2AProvider) CancelTask(_ context.Context, _ A2ATaskQuery) (A2ATaskResult, error) {
	return A2ATaskResult{Status: "canceled"}, nil
}

func (s *scriptedA2AProvider) GetCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getCalls
}

type inMemoryAsyncTaskStateStore struct {
	mu    sync.Mutex
	tasks []AsyncTask
}

func (s *inMemoryAsyncTaskStateStore) Load() ([]AsyncTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AsyncTask, 0, len(s.tasks))
	for _, item := range s.tasks {
		out = append(out, cloneAsyncTask(item))
	}
	return out, nil
}

func (s *inMemoryAsyncTaskStateStore) Save(tasks []AsyncTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AsyncTask, 0, len(tasks))
	for _, item := range tasks {
		out = append(out, cloneAsyncTask(item))
	}
	s.tasks = out
	return nil
}

func TestA2ATrackingWindowExhaustionPausesWithoutFailure(t *testing.T) {
	manager := newAsyncTaskManager(nil, "", time.Now)
	err := manager.SetA2ATrackingPolicy(A2ATrackingPolicy{
		InitialInterval:      5 * time.Millisecond,
		MaxInterval:          5 * time.Millisecond,
		MaxTrackingDuration:  12 * time.Millisecond,
		MaxConsecutiveErrors: 2,
		MinReconcileInterval: 0,
		MaxTrackingRenewals:  1,
	})
	if err != nil {
		t.Fatalf("SetA2ATrackingPolicy failed: %v", err)
	}
	manager.SetA2AProvider(&scriptedA2AProvider{
		sendResult: A2ATaskResult{AgentID: "codex-local", TaskID: "remote-1", Status: "working"},
		getSteps: []a2aGetStep{
			{result: A2ATaskResult{Status: "working"}},
		},
	})
	task, err := manager.Submit(context.Background(), AsyncTaskSubmitInput{
		TaskType:   asyncTaskTypeA2A,
		Request:    "long running",
		AgentID:    "codex-local",
		AgentInput: "run",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	paused := waitTaskUntil(t, manager, task.ID, 2*time.Second, func(task AsyncTask) bool {
		return task.TrackerState == asyncTaskTrackerStatePaused
	})
	if paused.Status != asyncTaskStatusWorking {
		t.Fatalf("expected working status after tracking pause, got %+v", paused)
	}
	if paused.TrackerReason != asyncTaskTrackerReasonWindowExhausted {
		t.Fatalf("expected tracking window exhausted reason, got %+v", paused)
	}
	if paused.TrackingRenewals < 1 || paused.LastRenewedAt.IsZero() {
		t.Fatalf("expected renewal evidence, got %+v", paused)
	}
	if !hasTaskLog(paused, "tracking_window_exhausted") {
		t.Fatalf("expected tracking window evidence in logs, got %+v", paused.Logs)
	}
	if paused.Error != "" {
		t.Fatalf("expected no terminal error for paused task, got %+v", paused)
	}
}

func TestA2ATerminalStatusFromPolling(t *testing.T) {
	manager := newAsyncTaskManager(nil, "", time.Now)
	manager.SetA2AProvider(&scriptedA2AProvider{
		sendResult: A2ATaskResult{AgentID: "codex-local", TaskID: "remote-2", Status: "working"},
		getSteps: []a2aGetStep{
			{result: A2ATaskResult{Status: "completed", AgentID: "codex-local", TaskID: "remote-2"}},
		},
	})
	task, err := manager.Submit(context.Background(), AsyncTaskSubmitInput{
		TaskType:   asyncTaskTypeA2A,
		Request:    "finish quickly",
		AgentID:    "codex-local",
		AgentInput: "run",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	done := waitTaskUntil(t, manager, task.ID, 4*time.Second, func(task AsyncTask) bool {
		return isAsyncTaskTerminal(task.Status)
	})
	if done.Status != asyncTaskStatusSucceeded {
		t.Fatalf("expected succeeded, got %+v", done)
	}
}

func TestA2ATrackerRecoversFromPersistedState(t *testing.T) {
	store := &inMemoryAsyncTaskStateStore{}
	now := time.Now().Truncate(time.Second)
	if err := store.Save([]AsyncTask{{
		ID:           "async_20260228_111648_1",
		TaskType:     asyncTaskTypeA2A,
		Status:       asyncTaskStatusWorking,
		TrackerState: asyncTaskTrackerStatePaused,
		AgentID:      "codex-local",
		RemoteTaskID: "remote-3",
		Request:      "recover",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}); err != nil {
		t.Fatalf("seed store failed: %v", err)
	}
	manager := newAsyncTaskManager(nil, "", time.Now)
	if err := manager.BindStateStore(store); err != nil {
		t.Fatalf("BindStateStore failed: %v", err)
	}
	manager.SetA2AProvider(&scriptedA2AProvider{
		sendResult: A2ATaskResult{Status: "working"},
		getSteps: []a2aGetStep{
			{result: A2ATaskResult{Status: "completed", AgentID: "codex-local", TaskID: "remote-3"}},
		},
	})
	done := waitTaskUntil(t, manager, "async_20260228_111648_1", 4*time.Second, func(task AsyncTask) bool {
		return isAsyncTaskTerminal(task.Status)
	})
	if done.Status != asyncTaskStatusSucceeded {
		t.Fatalf("expected recovered task succeeded, got %+v", done)
	}
}

func TestAsyncTaskGetReconcileAdvancesToTerminal(t *testing.T) {
	manager := newAsyncTaskManager(nil, "", time.Now)
	err := manager.SetA2ATrackingPolicy(A2ATrackingPolicy{
		InitialInterval:      5 * time.Millisecond,
		MaxInterval:          10 * time.Millisecond,
		MaxTrackingDuration:  30 * time.Second,
		MaxConsecutiveErrors: 1,
		MinReconcileInterval: 0,
		MaxTrackingRenewals:  2,
	})
	if err != nil {
		t.Fatalf("SetA2ATrackingPolicy failed: %v", err)
	}
	manager.SetA2AProvider(&scriptedA2AProvider{
		sendResult: A2ATaskResult{AgentID: "codex-local", TaskID: "remote-4", Status: "working"},
		getSteps: []a2aGetStep{
			{err: errors.New("temporary eof")},
			{result: A2ATaskResult{Status: "completed", AgentID: "codex-local", TaskID: "remote-4"}},
		},
	})
	task, err := manager.Submit(context.Background(), AsyncTaskSubmitInput{
		TaskType:   asyncTaskTypeA2A,
		Request:    "reconcile",
		AgentID:    "codex-local",
		AgentInput: "run",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	waitTaskUntil(t, manager, task.ID, 2*time.Second, func(task AsyncTask) bool {
		return task.TrackerState == asyncTaskTrackerStatePaused
	})
	reconciled, err := manager.Get(AsyncTaskGetInput{TaskID: task.ID})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if reconciled.Status != asyncTaskStatusSucceeded {
		t.Fatalf("expected reconcile advances to succeeded, got %+v", reconciled)
	}
}

func TestSetA2ATrackingPolicyValidation(t *testing.T) {
	manager := newAsyncTaskManager(nil, "", time.Now)
	err := manager.SetA2ATrackingPolicy(A2ATrackingPolicy{
		InitialInterval:      2 * time.Second,
		MaxInterval:          1 * time.Second,
		MaxTrackingDuration:  3 * time.Second,
		MaxConsecutiveErrors: 1,
		MinReconcileInterval: 0,
		MaxTrackingRenewals:  1,
	})
	if err == nil {
		t.Fatalf("expected invalid policy error")
	}
}

func TestA2ATrackingAutoRenewKeepsNonTerminal(t *testing.T) {
	manager := newAsyncTaskManager(nil, "", time.Now)
	err := manager.SetA2ATrackingPolicy(A2ATrackingPolicy{
		InitialInterval:      5 * time.Millisecond,
		MaxInterval:          5 * time.Millisecond,
		MaxTrackingDuration:  10 * time.Millisecond,
		MaxConsecutiveErrors: 2,
		MinReconcileInterval: 0,
		MaxTrackingRenewals:  4,
	})
	if err != nil {
		t.Fatalf("SetA2ATrackingPolicy failed: %v", err)
	}
	manager.SetA2AProvider(&scriptedA2AProvider{
		sendResult: A2ATaskResult{AgentID: "codex-local", TaskID: "remote-renew", Status: "working"},
		getSteps: []a2aGetStep{
			{result: A2ATaskResult{Status: "working"}},
		},
	})
	task, err := manager.Submit(context.Background(), AsyncTaskSubmitInput{
		TaskType:   asyncTaskTypeA2A,
		Request:    "renew",
		AgentID:    "codex-local",
		AgentInput: "run",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	renewed := waitTaskUntil(t, manager, task.ID, 2*time.Second, func(task AsyncTask) bool {
		return task.TrackingRenewals >= 1
	})
	if isAsyncTaskTerminal(renewed.Status) {
		t.Fatalf("expected non-terminal status after renewal, got %+v", renewed)
	}
	if renewed.LastRenewedAt.IsZero() {
		t.Fatalf("expected last renewed timestamp, got %+v", renewed)
	}
}

func TestAsyncTaskGetReconcileDebounce(t *testing.T) {
	manager := newAsyncTaskManager(nil, "", time.Now)
	err := manager.SetA2ATrackingPolicy(A2ATrackingPolicy{
		InitialInterval:      5 * time.Millisecond,
		MaxInterval:          10 * time.Millisecond,
		MaxTrackingDuration:  10 * time.Second,
		MaxConsecutiveErrors: 1,
		MinReconcileInterval: 1 * time.Second,
		MaxTrackingRenewals:  2,
	})
	if err != nil {
		t.Fatalf("SetA2ATrackingPolicy failed: %v", err)
	}
	provider := &scriptedA2AProvider{
		sendResult: A2ATaskResult{AgentID: "codex-local", TaskID: "remote-5", Status: "working"},
		getSteps: []a2aGetStep{
			{err: errors.New("temporary eof")},
			{result: A2ATaskResult{Status: "working", AgentID: "codex-local", TaskID: "remote-5"}},
		},
	}
	manager.SetA2AProvider(provider)
	task, err := manager.Submit(context.Background(), AsyncTaskSubmitInput{
		TaskType:   asyncTaskTypeA2A,
		Request:    "debounce",
		AgentID:    "codex-local",
		AgentInput: "run",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	waitTaskUntil(t, manager, task.ID, 2*time.Second, func(task AsyncTask) bool {
		return task.TrackerState == asyncTaskTrackerStatePaused
	})
	first, err := manager.Get(AsyncTaskGetInput{TaskID: task.ID})
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}
	if first.ReconcileSkippedByDebounce {
		t.Fatalf("first reconcile should not be debounced")
	}
	callsAfterFirst := provider.GetCalls()
	second, err := manager.Get(AsyncTaskGetInput{TaskID: task.ID})
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}
	if !second.ReconcileSkippedByDebounce {
		t.Fatalf("second reconcile should be debounced, got %+v", second)
	}
	if provider.GetCalls() != callsAfterFirst {
		t.Fatalf("expected no extra remote call during debounce window")
	}
}

func waitTaskUntil(t *testing.T, manager *AsyncTaskManager, taskID string, timeout time.Duration, predicate func(AsyncTask) bool) AsyncTask {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, ok := readTaskSnapshot(manager, taskID)
		if ok && predicate(task) {
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
	task, _ := readTaskSnapshot(manager, taskID)
	t.Fatalf("task %s did not reach expected state, latest=%+v", taskID, task)
	return AsyncTask{}
}

func readTaskSnapshot(manager *AsyncTaskManager, taskID string) (AsyncTask, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	state := manager.tasks[taskID]
	if state == nil {
		return AsyncTask{}, false
	}
	return cloneAsyncTask(state.task), true
}

func hasTaskLog(task AsyncTask, needle string) bool {
	for _, item := range task.Logs {
		if strings.Contains(item.Message, needle) {
			return true
		}
	}
	return false
}
