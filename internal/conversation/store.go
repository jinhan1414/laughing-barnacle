package conversation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ToolCall struct {
	ID        string    `json:"id,omitempty"`
	Name      string    `json:"name"`
	Arguments string    `json:"arguments,omitempty"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Message is one conversation record kept in memory.
type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Store holds one global conversation (no session concept).
type Store struct {
	mu       sync.RWMutex
	path     string
	summary  string
	messages []Message
}

func NewStore() *Store {
	return &Store{}
}

func NewStoreWithFile(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("conversation file path is required")
	}
	s := &Store{path: path}
	if err := s.loadFromFile(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Append(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = append(s.messages, Message{
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	})
	_ = s.persistLocked()
}

func (s *Store) SetLatestUserToolCalls(toolCalls []ToolCall) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.messages) == 0 || s.messages[len(s.messages)-1].Role != "user" {
		return fmt.Errorf("no pending user message")
	}
	s.messages[len(s.messages)-1].ToolCalls = normalizeToolCalls(toolCalls)
	_ = s.persistLocked()
	return nil
}

func (s *Store) Snapshot() (string, []Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.summary, cloneMessages(s.messages)
}

func (s *Store) SetSummaryAndTrim(summary string, keepRecent int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.summary = summary
	if keepRecent < 0 {
		keepRecent = 0
	}
	if len(s.messages) <= keepRecent {
		_ = s.persistLocked()
		return
	}
	s.messages = append([]Message(nil), s.messages[len(s.messages)-keepRecent:]...)
	_ = s.persistLocked()
}

func (s *Store) loadFromFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create conversation dir: %w", err)
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.summary = ""
			s.messages = nil
			return s.persistLocked()
		}
		return fmt.Errorf("read conversation file: %w", err)
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		s.summary = ""
		s.messages = nil
		return nil
	}

	var payload struct {
		Summary  string    `json:"summary"`
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("decode conversation file: %w", err)
	}

	s.summary = payload.Summary
	s.messages = cloneMessages(payload.Messages)
	return s.persistLocked()
}

func (s *Store) persistLocked() error {
	if strings.TrimSpace(s.path) == "" {
		return nil
	}

	payload := struct {
		Summary  string    `json:"summary"`
		Messages []Message `json:"messages"`
	}{
		Summary:  s.summary,
		Messages: s.messages,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode conversation: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create conversation dir: %w", err)
	}

	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp conversation: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		return fmt.Errorf("rename conversation file: %w", err)
	}
	return nil
}

func cloneMessages(in []Message) []Message {
	if len(in) == 0 {
		return nil
	}
	out := make([]Message, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].ToolCalls = cloneToolCalls(in[i].ToolCalls)
	}
	return out
}

func cloneToolCalls(in []ToolCall) []ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]ToolCall, len(in))
	copy(out, in)
	return out
}

func normalizeToolCalls(in []ToolCall) []ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(in))
	for _, call := range in {
		call.ID = strings.TrimSpace(call.ID)
		call.Name = strings.TrimSpace(call.Name)
		call.Arguments = strings.TrimSpace(call.Arguments)
		call.Result = strings.TrimSpace(call.Result)
		call.Error = strings.TrimSpace(call.Error)
		if call.Name == "" {
			call.Name = "(unknown)"
		}
		if call.Arguments == "" {
			call.Arguments = "{}"
		}
		if call.CreatedAt.IsZero() {
			call.CreatedAt = time.Now()
		}
		out = append(out, call)
	}
	return out
}
