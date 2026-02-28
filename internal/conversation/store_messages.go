package conversation

import (
	"fmt"
	"strings"
	"time"
)

func (s *Store) Append(role, content string) {
	s.AppendWithUsage(role, content, nil)
}

func (s *Store) AppendAssistant(content string, usage *TokenUsage) {
	s.AppendWithUsage("assistant", content, usage)
}

func (s *Store) AppendWithUsage(role, content string, usage *TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = append(s.messages, Message{
		Role:      role,
		Content:   content,
		Usage:     normalizeTokenUsage(usage),
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
	return s.persistLocked()
}

func (s *Store) Snapshot() (string, []Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.summary, cloneMessages(s.messages)
}

func (s *Store) SnapshotWithEvents() (string, []Message, []Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.summary, cloneMessages(s.messages), cloneEvents(s.events)
}

func (s *Store) AppendEvent(eventType, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	eventType = strings.TrimSpace(eventType)
	content = strings.TrimSpace(content)
	if eventType == "" || content == "" {
		return
	}
	s.events = append(s.events, Event{
		Type:      eventType,
		Content:   content,
		CreatedAt: time.Now(),
	})
	if len(s.events) > maxArchiveEvents {
		s.events = append([]Event(nil), s.events[len(s.events)-maxArchiveEvents:]...)
	}
	_ = s.persistLocked()
}

func (s *Store) SetSummaryAndTrim(summary string, keepRecent int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	summary = strings.TrimSpace(summary)
	if keepRecent < 0 {
		keepRecent = 0
	}
	if len(s.messages) <= keepRecent {
		s.summary = summary
		_ = s.persistLocked()
		return
	}

	trimmed := cloneMessages(s.messages[:len(s.messages)-keepRecent])
	s.messages = append([]Message(nil), s.messages[len(s.messages)-keepRecent:]...)

	if ref, err := s.createArchiveLocked(trimmed); err == nil {
		summary = appendArchiveRefToSummary(summary, ref)
	}
	s.summary = summary
	_ = s.persistLocked()
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.summary = ""
	s.messages = nil
	s.events = nil
	if err := s.persistLocked(); err != nil {
		return err
	}
	return s.deleteAsyncTaskStateLocked()
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
