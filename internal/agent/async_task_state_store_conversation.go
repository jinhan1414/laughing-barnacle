package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"laughing-barnacle/internal/conversation"
)

type conversationAsyncTaskStateStore struct {
	store *conversation.Store
}

func NewConversationAsyncTaskStateStore(store *conversation.Store) AsyncTaskStateStore {
	if store == nil {
		return nil
	}
	return &conversationAsyncTaskStateStore{store: store}
}

func (s *conversationAsyncTaskStateStore) Load() ([]AsyncTask, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	raw, err := s.store.LoadAsyncTaskState()
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var out []AsyncTask
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode async task state: %w", err)
	}
	return normalizePersistedTasks(out), nil
}

func (s *conversationAsyncTaskStateStore) Save(tasks []AsyncTask) error {
	if s == nil || s.store == nil {
		return nil
	}
	normalized := normalizePersistedTasks(tasks)
	if len(normalized) == 0 {
		return s.store.SaveAsyncTaskState(nil)
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("encode async task state: %w", err)
	}
	return s.store.SaveAsyncTaskState(raw)
}

func normalizePersistedTasks(tasks []AsyncTask) []AsyncTask {
	if len(tasks) == 0 {
		return nil
	}
	out := make([]AsyncTask, 0, len(tasks))
	for _, task := range tasks {
		task.ID = strings.TrimSpace(task.ID)
		if task.ID == "" {
			continue
		}
		out = append(out, cloneAsyncTask(task))
	}
	return out
}
