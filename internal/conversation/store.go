package conversation

import (
	"sync"
	"time"
)

// Message is one conversation record kept in memory.
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Store holds one global conversation (no session concept).
type Store struct {
	mu       sync.RWMutex
	summary  string
	messages []Message
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Append(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = append(s.messages, Message{
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	})
}

func (s *Store) Snapshot() (string, []Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copyMessages := make([]Message, len(s.messages))
	copy(copyMessages, s.messages)
	return s.summary, copyMessages
}

func (s *Store) SetSummaryAndTrim(summary string, keepRecent int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.summary = summary
	if keepRecent < 0 {
		keepRecent = 0
	}
	if len(s.messages) <= keepRecent {
		return
	}
	s.messages = append([]Message(nil), s.messages[len(s.messages)-keepRecent:]...)
}
