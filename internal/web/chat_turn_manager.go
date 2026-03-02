package web

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
)

const (
	chatTurnEventType        = "turn_status"
	chatTurnExecutionTimeout = 15 * time.Minute
	maxRetainedChatTurns     = 200
)

type chatTurnManager struct {
	mu           sync.Mutex
	store        *conversation.Store
	agent        *agent.Agent
	nowFn        func() time.Time
	seq          int64
	order        []string
	turns        map[string]conversation.ChatTurn
	processing   bool
	activeCancel context.CancelFunc
}

func newChatTurnManager(store *conversation.Store, agentSvc *agent.Agent) (*chatTurnManager, error) {
	manager := &chatTurnManager{
		store: store,
		agent: agentSvc,
		nowFn: time.Now,
		turns: make(map[string]conversation.ChatTurn),
	}
	if err := manager.load(); err != nil {
		return nil, err
	}
	manager.ensureWorker()
	return manager, nil
}

func (m *chatTurnManager) Submit(content string) (conversation.ChatTurn, error) {
	content = strings.TrimSpace(content)
	if m == nil || m.agent == nil || m.store == nil {
		return conversation.ChatTurn{}, fmt.Errorf("chat turn manager unavailable")
	}
	if content == "" {
		return conversation.ChatTurn{}, fmt.Errorf("message is required")
	}
	now := m.nowFn().UTC()
	m.mu.Lock()
	turn := conversation.ChatTurn{
		ID:         m.nextIDLocked("turn", now),
		MessageID:  m.nextIDLocked("msg", now),
		Content:    content,
		Status:     conversation.ChatTurnStatusQueued,
		AcceptedAt: now,
		UpdatedAt:  now,
	}
	m.turns[turn.ID] = turn
	m.order = append(m.order, turn.ID)
	m.trimLocked()
	err := m.persistLocked()
	m.mu.Unlock()
	if err != nil {
		return conversation.ChatTurn{}, err
	}
	m.store.AppendEvent(chatTurnEventType, formatChatTurnStatus(turn))
	m.ensureWorker()
	return turn, nil
}

func (m *chatTurnManager) ListActive() []conversation.ChatTurn {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]conversation.ChatTurn, 0, len(m.order))
	for _, id := range m.order {
		turn, ok := m.turns[id]
		if !ok || isChatTurnTerminal(turn.Status) {
			continue
		}
		out = append(out, turn)
	}
	return out
}

func (m *chatTurnManager) Reset() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	cancel := m.activeCancel
	m.turns = make(map[string]conversation.ChatTurn)
	m.order = nil
	m.processing = false
	m.activeCancel = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return m.store.SaveChatTurnState(nil)
}

func (m *chatTurnManager) ensureWorker() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.processing || !m.hasQueuedLocked() {
		m.mu.Unlock()
		return
	}
	m.processing = true
	m.mu.Unlock()
	go m.runLoop()
}

func (m *chatTurnManager) runLoop() {
	for {
		turn, ok := m.startNextTurn()
		if !ok {
			m.stopWorker()
			return
		}
		err := m.executeTurn(turn)
		m.finishTurn(turn.ID, err)
	}
}

func (m *chatTurnManager) executeTurn(turn conversation.ChatTurn) error {
	ctx, cancel := context.WithTimeout(context.Background(), chatTurnExecutionTimeout)
	m.mu.Lock()
	m.activeCancel = cancel
	m.mu.Unlock()
	defer func() {
		cancel()
		m.mu.Lock()
		m.activeCancel = nil
		m.mu.Unlock()
	}()
	_, err := m.agent.HandleUserMessage(ctx, turn.Content)
	return err
}

func (m *chatTurnManager) startNextTurn() (conversation.ChatTurn, bool) {
	now := m.nowFn().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range m.order {
		turn, ok := m.turns[id]
		if !ok || turn.Status != conversation.ChatTurnStatusQueued {
			continue
		}
		turn.Status = conversation.ChatTurnStatusWorking
		turn.StartedAt = now
		turn.UpdatedAt = now
		m.turns[id] = turn
		if err := m.persistLocked(); err != nil {
			turn.Status = conversation.ChatTurnStatusFailed
			turn.Error = err.Error()
			turn.UpdatedAt = now
			m.turns[id] = turn
		}
		go m.store.AppendEvent(chatTurnEventType, formatChatTurnStatus(turn))
		return turn, true
	}
	return conversation.ChatTurn{}, false
}

func (m *chatTurnManager) finishTurn(turnID string, runErr error) {
	now := m.nowFn().UTC()
	m.mu.Lock()
	turn, ok := m.turns[turnID]
	if !ok {
		m.mu.Unlock()
		return
	}
	turn.CompletedAt = now
	turn.UpdatedAt = now
	if runErr != nil {
		turn.Status = conversation.ChatTurnStatusFailed
		turn.Error = strings.TrimSpace(runErr.Error())
	} else {
		turn.Status = conversation.ChatTurnStatusCompleted
		turn.Error = ""
	}
	m.turns[turnID] = turn
	_ = m.persistLocked()
	m.mu.Unlock()
	m.store.AppendEvent(chatTurnEventType, formatChatTurnStatus(turn))
}

func (m *chatTurnManager) stopWorker() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processing = false
	if m.hasQueuedLocked() {
		m.processing = true
		go m.runLoop()
	}
}

func (m *chatTurnManager) hasQueuedLocked() bool {
	for _, id := range m.order {
		turn, ok := m.turns[id]
		if ok && turn.Status == conversation.ChatTurnStatusQueued {
			return true
		}
	}
	return false
}

func (m *chatTurnManager) trimLocked() {
	if len(m.order) <= maxRetainedChatTurns {
		return
	}
	keep := m.order[:0]
	for _, id := range m.order {
		turn, ok := m.turns[id]
		if !ok {
			continue
		}
		if len(keep) >= maxRetainedChatTurns && isChatTurnTerminal(turn.Status) {
			delete(m.turns, id)
			continue
		}
		keep = append(keep, id)
	}
	m.order = append([]string(nil), keep...)
}

func (m *chatTurnManager) persistLocked() error {
	turns := make([]conversation.ChatTurn, 0, len(m.order))
	for _, id := range m.order {
		if turn, ok := m.turns[id]; ok {
			turns = append(turns, turn)
		}
	}
	return m.store.SaveChatTurnState(turns)
}

func (m *chatTurnManager) load() error {
	turns, err := m.store.LoadChatTurnState()
	if err != nil {
		return err
	}
	now := m.nowFn().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, turn := range turns {
		if turn.Status == conversation.ChatTurnStatusWorking {
			turn.Status = conversation.ChatTurnStatusQueued
			turn.StartedAt = time.Time{}
			turn.UpdatedAt = now
		}
		m.turns[turn.ID] = turn
		m.order = append(m.order, turn.ID)
	}
	return m.persistLocked()
}

func (m *chatTurnManager) nextIDLocked(prefix string, now time.Time) string {
	m.seq++
	return fmt.Sprintf("%s_%s_%d", prefix, now.Format("20060102_150405_000000"), m.seq)
}

func isChatTurnTerminal(status string) bool {
	return status == conversation.ChatTurnStatusCompleted || status == conversation.ChatTurnStatusFailed
}

func formatChatTurnStatus(turn conversation.ChatTurn) string {
	parts := []string{
		"turn_id=" + strings.TrimSpace(turn.ID),
		"message_id=" + strings.TrimSpace(turn.MessageID),
		"status=" + strings.TrimSpace(turn.Status),
		"brief=" + trimChatTurnText(turn.Content, 80),
	}
	if text := strings.TrimSpace(turn.Error); text != "" {
		parts = append(parts, "error="+trimChatTurnText(text, 160))
	}
	return strings.Join(parts, " | ")
}

func trimChatTurnText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return strings.TrimSpace(string(runes[:limit-3])) + "..."
}
