package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"laughing-barnacle/internal/conversation"
)

type conversationAutonomousRunStateStore struct {
	store *conversation.Store
}

func NewConversationAutonomousRunStateStore(store *conversation.Store) AutonomousRunStateStore {
	if store == nil {
		return nil
	}
	return &conversationAutonomousRunStateStore{store: store}
}

func (s *conversationAutonomousRunStateStore) Load() ([]AutonomousRun, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	raw, err := s.store.LoadAutonomousRunState()
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var out []AutonomousRun
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode autonomous run state: %w", err)
	}
	return normalizePersistedRuns(out), nil
}

func (s *conversationAutonomousRunStateStore) Save(runs []AutonomousRun) error {
	if s == nil || s.store == nil {
		return nil
	}
	normalized := normalizePersistedRuns(runs)
	if len(normalized) == 0 {
		return s.store.SaveAutonomousRunState(nil)
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("encode autonomous run state: %w", err)
	}
	return s.store.SaveAutonomousRunState(raw)
}

func normalizePersistedRuns(runs []AutonomousRun) []AutonomousRun {
	if len(runs) == 0 {
		return nil
	}
	out := make([]AutonomousRun, 0, len(runs))
	for _, run := range runs {
		run.ID = strings.TrimSpace(run.ID)
		if run.ID == "" {
			continue
		}
		out = append(out, cloneAutonomousRun(run))
	}
	return out
}
