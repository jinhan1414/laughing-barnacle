package conversation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	ChatTurnStatusQueued    = "queued"
	ChatTurnStatusWorking   = "working"
	ChatTurnStatusCompleted = "completed"
	ChatTurnStatusFailed    = "failed"
)

type ChatTurn struct {
	ID          string    `json:"id"`
	MessageID   string    `json:"message_id"`
	Content     string    `json:"content"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
	AcceptedAt  time.Time `json:"accepted_at"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Store) LoadChatTurnState() ([]ChatTurn, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, nil
	}
	var turns []ChatTurn
	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(bucketMeta))
		if meta == nil {
			return fmt.Errorf("conversation db schema missing")
		}
		raw := meta.Get([]byte(metaChatTurnState))
		if len(raw) == 0 {
			return nil
		}
		return json.Unmarshal(raw, &turns)
	})
	if err != nil {
		return nil, err
	}
	return normalizeChatTurns(turns), nil
}

func (s *Store) SaveChatTurnState(turns []ChatTurn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	normalized := normalizeChatTurns(turns)
	if len(normalized) == 0 {
		return s.deleteChatTurnStateLocked()
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("encode chat turn state: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(bucketMeta))
		if meta == nil {
			return fmt.Errorf("conversation db schema missing")
		}
		return meta.Put([]byte(metaChatTurnState), raw)
	})
}

func normalizeChatTurns(turns []ChatTurn) []ChatTurn {
	if len(turns) == 0 {
		return nil
	}
	out := make([]ChatTurn, 0, len(turns))
	for _, turn := range turns {
		turn.ID = strings.TrimSpace(turn.ID)
		turn.MessageID = strings.TrimSpace(turn.MessageID)
		turn.Content = strings.TrimSpace(turn.Content)
		turn.Status = normalizeChatTurnStatus(turn.Status)
		turn.Error = strings.TrimSpace(turn.Error)
		if turn.ID == "" || turn.MessageID == "" || turn.Content == "" {
			continue
		}
		if turn.AcceptedAt.IsZero() {
			turn.AcceptedAt = time.Now()
		}
		if turn.UpdatedAt.IsZero() {
			turn.UpdatedAt = turn.AcceptedAt
		}
		out = append(out, turn)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeChatTurnStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case ChatTurnStatusWorking:
		return ChatTurnStatusWorking
	case ChatTurnStatusCompleted:
		return ChatTurnStatusCompleted
	case ChatTurnStatusFailed:
		return ChatTurnStatusFailed
	default:
		return ChatTurnStatusQueued
	}
}
