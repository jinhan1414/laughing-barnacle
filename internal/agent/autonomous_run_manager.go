package agent

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type autonomousRunState struct {
	run AutonomousRun
}

type AutonomousRunManager struct {
	mu         sync.RWMutex
	nowFn      func() time.Time
	seq        int64
	order      []string
	runs       map[string]*autonomousRunState
	stateStore AutonomousRunStateStore
	onStat     func(AutonomousRun)
}

func newAutonomousRunManager(nowFn func() time.Time) *AutonomousRunManager {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &AutonomousRunManager{
		nowFn: nowFn,
		runs:  make(map[string]*autonomousRunState),
		order: make([]string, 0, 8),
	}
}

func (m *AutonomousRunManager) BindStateStore(store AutonomousRunStateStore) error {
	if store == nil {
		return fmt.Errorf("autonomous run state store is required")
	}
	m.mu.Lock()
	m.stateStore = store
	m.mu.Unlock()
	return m.loadSnapshotFromStore()
}

func (m *AutonomousRunManager) SetHooks(onStatus func(AutonomousRun)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onStat = onStatus
}

func (m *AutonomousRunManager) Checkpoint(input AutonomousRunCheckpointInput) (AutonomousRun, error) {
	if err := validateRunCheckpoint(input); err != nil {
		return AutonomousRun{}, err
	}
	now := m.nowFn().Truncate(time.Second)
	m.mu.Lock()
	defer m.mu.Unlock()

	var state *autonomousRunState
	if strings.TrimSpace(input.RunID) == "" {
		runID := m.nextRunIDLocked(now)
		state = &autonomousRunState{run: AutonomousRun{
			ID:        runID,
			Goal:      strings.TrimSpace(input.Goal),
			CreatedAt: now,
		}}
		m.runs[runID] = state
		m.order = append(m.order, runID)
		m.trimRunEntriesLocked()
	} else {
		state = m.runs[strings.TrimSpace(input.RunID)]
		if state == nil {
			return AutonomousRun{}, fmt.Errorf("run %q not found", strings.TrimSpace(input.RunID))
		}
	}
	applyRunCheckpointLocked(state, input, now)
	m.persistSnapshotLocked()
	snapshot := cloneAutonomousRun(state.run)
	go m.emitStatusChange(snapshot)
	return snapshot, nil
}

func (m *AutonomousRunManager) Get(runID string) (AutonomousRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.runs[strings.TrimSpace(runID)]
	if state == nil {
		return AutonomousRun{}, fmt.Errorf("run %q not found", strings.TrimSpace(runID))
	}
	return cloneAutonomousRun(state.run), nil
}

func (m *AutonomousRunManager) ListRuns() []AutonomousRun {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]AutonomousRun, 0, len(m.order))
	for i := len(m.order) - 1; i >= 0; i-- {
		if state := m.runs[m.order[i]]; state != nil {
			out = append(out, cloneAutonomousRun(state.run))
		}
	}
	return out
}

func (m *AutonomousRunManager) ListIndexLines(limit int, now time.Time) []string {
	items := m.ListRuns()
	out := make([]string, 0, len(items))
	for _, run := range items {
		if isAutonomousRunTerminal(run.Status) && !sameLocalDay(run.CreatedAt.Local(), now.Local()) {
			continue
		}
		out = append(out, fmt.Sprintf(
			"run_id=%s | status=%s | step=%s | waiting=%s | updated_at=%s | goal=%s",
			run.ID,
			run.Status,
			run.CurrentStep,
			safeOrEmpty(run.WaitingType),
			run.UpdatedAt.Local().Format("2006-01-02 15:04:05"),
			trimRunes(run.Goal, 80),
		))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (m *AutonomousRunManager) MatchWaitingAsyncTask(taskID string) []AutonomousRun {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]AutonomousRun, 0, 1)
	for _, id := range m.order {
		state := m.runs[id]
		if state == nil {
			continue
		}
		run := state.run
		if run.Status != autonomousRunStatusWaitingAsync || run.WaitingType != autonomousRunWaitingTypeAsync {
			continue
		}
		if strings.TrimSpace(run.WaitingRef) != taskID {
			continue
		}
		out = append(out, cloneAutonomousRun(run))
	}
	return out
}

func (m *AutonomousRunManager) nextRunIDLocked(now time.Time) string {
	m.seq++
	return "run_" + now.Format("20060102_150405") + "_" + strconv.FormatInt(m.seq, 10)
}

func (m *AutonomousRunManager) emitStatusChange(run AutonomousRun) {
	m.mu.RLock()
	hook := m.onStat
	m.mu.RUnlock()
	if hook != nil {
		hook(run)
	}
}

func (m *AutonomousRunManager) trimRunEntriesLocked() {
	if len(m.order) <= maxAutonomousRunsRetained {
		return
	}
	drop := len(m.order) - maxAutonomousRunsRetained
	for i := 0; i < drop; i++ {
		delete(m.runs, m.order[i])
	}
	m.order = append([]string(nil), m.order[drop:]...)
}

func (m *AutonomousRunManager) persistSnapshotLocked() {
	if m.stateStore == nil {
		return
	}
	snapshot := make([]AutonomousRun, 0, len(m.order))
	for _, id := range m.order {
		if state := m.runs[id]; state != nil {
			snapshot = append(snapshot, cloneAutonomousRun(state.run))
		}
	}
	_ = m.stateStore.Save(snapshot)
}

func (m *AutonomousRunManager) loadSnapshotFromStore() error {
	m.mu.RLock()
	store := m.stateStore
	m.mu.RUnlock()
	if store == nil {
		return nil
	}
	runs, err := store.Load()
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs = make(map[string]*autonomousRunState, len(runs))
	m.order = make([]string, 0, len(runs))
	for _, run := range runs {
		run = cloneAutonomousRun(run)
		m.runs[run.ID] = &autonomousRunState{run: run}
		m.order = append(m.order, run.ID)
	}
	return nil
}

func validateRunCheckpoint(input AutonomousRunCheckpointInput) error {
	status := strings.TrimSpace(input.Status)
	step := strings.TrimSpace(input.CurrentStep)
	if status == "" {
		return fmt.Errorf("status is required")
	}
	if step == "" {
		return fmt.Errorf("current_step is required")
	}
	switch status {
	case autonomousRunStatusRunning, autonomousRunStatusWaitingAsync, autonomousRunStatusWaitingHuman, autonomousRunStatusCompleted, autonomousRunStatusFailed:
	default:
		return fmt.Errorf("unsupported status %q", status)
	}
	if strings.TrimSpace(input.RunID) == "" && strings.TrimSpace(input.Goal) == "" {
		return fmt.Errorf("goal is required when creating a run")
	}
	if status == autonomousRunStatusWaitingAsync && strings.TrimSpace(input.WaitingRef) == "" {
		return fmt.Errorf("waiting_ref is required when status=waiting_async")
	}
	return nil
}

func applyRunCheckpointLocked(state *autonomousRunState, input AutonomousRunCheckpointInput, now time.Time) {
	if state == nil {
		return
	}
	run := &state.run
	if text := strings.TrimSpace(input.Goal); text != "" {
		run.Goal = text
	}
	run.Status = strings.TrimSpace(input.Status)
	run.CurrentStep = strings.TrimSpace(input.CurrentStep)
	run.LastEventType = strings.TrimSpace(input.LastEventType)
	run.LastEventSummary = strings.TrimSpace(input.LastEventText)
	run.Error = strings.TrimSpace(input.Error)
	run.Context = mergeRunContext(run.Context, input.ContextPatch)
	switch run.Status {
	case autonomousRunStatusWaitingAsync:
		run.WaitingType = autonomousRunWaitingTypeAsync
		run.WaitingRef = strings.TrimSpace(input.WaitingRef)
	case autonomousRunStatusWaitingHuman:
		run.WaitingType = autonomousRunWaitingTypeHuman
		run.WaitingRef = strings.TrimSpace(input.WaitingRef)
	default:
		run.WaitingType = ""
		run.WaitingRef = ""
	}
	run.UpdatedAt = now
	appendRunStepLocked(run, input, now)
}

func appendRunStepLocked(run *AutonomousRun, input AutonomousRunCheckpointInput, now time.Time) {
	if run == nil {
		return
	}
	run.Steps = append(run.Steps, RunStep{
		Seq:         len(run.Steps) + 1,
		Step:        run.CurrentStep,
		Status:      run.Status,
		Summary:     strings.TrimSpace(input.StepSummary),
		WaitingType: run.WaitingType,
		WaitingRef:  run.WaitingRef,
		CreatedAt:   now,
	})
	if len(run.Steps) > maxAutonomousRunSteps {
		run.Steps = append([]RunStep(nil), run.Steps[len(run.Steps)-maxAutonomousRunSteps:]...)
	}
}

func mergeRunContext(base, patch map[string]any) map[string]any {
	if len(base) == 0 && len(patch) == 0 {
		return nil
	}
	out := cloneAnyMap(base)
	if out == nil {
		out = make(map[string]any, len(patch))
	}
	for key, value := range patch {
		out[strings.TrimSpace(key)] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneAutonomousRun(in AutonomousRun) AutonomousRun {
	out := in
	out.Context = cloneAnyMap(in.Context)
	if len(in.Steps) > 0 {
		out.Steps = append([]RunStep(nil), in.Steps...)
	}
	return out
}

func sameLocalDay(left, right time.Time) bool {
	return left.Year() == right.Year() &&
		left.Month() == right.Month() &&
		left.Day() == right.Day()
}
