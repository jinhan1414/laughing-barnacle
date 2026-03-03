package mcp

import (
	"strings"
	"time"
)

func (s *Store) GetProjectRootDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.Agent.Habits.ProjectRootDir)
}

func (s *Store) SetProjectRootDir(path string) error {
	path = strings.TrimSpace(path)
	if err := validateProjectRootDir(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Agent.Habits.ProjectRootDir = path
	s.cfg.Agent.Habits.UpdatedAt = time.Now()
	return s.persistLocked()
}
