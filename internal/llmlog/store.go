package llmlog

import (
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
	nextID  atomic.Int64
}

func NewStore(limit int) *Store {
	if limit <= 0 {
		limit = 500
	}
	return &Store{limit: limit, entries: make([]Entry, 0, limit)}
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
}

func (s *Store) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	return out
}
