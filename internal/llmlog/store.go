package llmlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Entry captures one real LLM call's input and output.
type Entry struct {
	ID         int64
	Time       time.Time
	Purpose    string
	Model      string
	Request    string
	Response   string
	Error      string
	StatusCode int
	DurationMS int64
}

// Store keeps in-memory LLM call logs for the log page.
type Store struct {
	mu      sync.RWMutex
	entries []Entry
	limit   int
	path    string
	nextID  atomic.Int64
}

func NewStore(limit int) *Store {
	if limit <= 0 {
		limit = 500
	}
	return &Store{limit: limit, entries: make([]Entry, 0, limit)}
}

func NewStoreWithFile(limit int, path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("llm log file path is required")
	}

	s := NewStore(limit)
	s.path = path
	if err := s.loadFromFile(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Add(e Entry) {
	e.ID = s.nextID.Add(1)
	if e.Time.IsZero() {
		e.Time = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = append([]Entry{e}, s.entries...)
	if len(s.entries) > s.limit {
		s.entries = s.entries[:s.limit]
	}
	_ = s.persistLocked()
}

func (s *Store) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

func (s *Store) loadFromFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create llm log dir: %w", err)
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.entries = make([]Entry, 0, s.limit)
			s.nextID.Store(0)
			return s.persistLocked()
		}
		return fmt.Errorf("read llm log file: %w", err)
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		s.entries = make([]Entry, 0, s.limit)
		s.nextID.Store(0)
		return nil
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("decode llm log file: %w", err)
	}
	if len(entries) > s.limit {
		entries = entries[:s.limit]
	}

	var maxID int64
	for _, entry := range entries {
		if entry.ID > maxID {
			maxID = entry.ID
		}
	}

	s.entries = entries
	s.nextID.Store(maxID)
	return s.persistLocked()
}

func (s *Store) persistLocked() error {
	if strings.TrimSpace(s.path) == "" {
		return nil
	}

	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encode llm logs: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create llm log dir: %w", err)
	}

	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp llm logs: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		return fmt.Errorf("rename llm log file: %w", err)
	}
	return nil
}
