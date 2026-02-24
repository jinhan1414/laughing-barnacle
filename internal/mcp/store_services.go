package mcp

import (
	"fmt"
	"strings"
	"time"
)

func (s *Store) ListServices() []Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneServices(s.cfg.MCP.Services)
}

func (s *Store) ListEnabledServices() []Service {
	all := s.ListServices()
	enabled := make([]Service, 0, len(all))
	for _, svc := range all {
		if svc.Enabled {
			enabled = append(enabled, svc)
		}
	}
	return enabled
}

func (s *Store) GetService(id string) (Service, bool) {
	id = strings.TrimSpace(id)

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, svc := range s.cfg.MCP.Services {
		if svc.ID == id {
			return cloneService(svc), true
		}
	}
	return Service{}, false
}

func (s *Store) UpsertService(service Service) error {
	service.ID = strings.TrimSpace(service.ID)
	service.Name = strings.TrimSpace(service.Name)
	service.Endpoint = strings.TrimSpace(service.Endpoint)
	service.Command = strings.TrimSpace(service.Command)
	service.Args = normalizeServiceArgs(service.Args)
	service.Transport = normalizeServiceTransport(service.Transport)
	service.AuthToken = strings.TrimSpace(service.AuthToken)
	service.ToolStates = normalizeServiceToolStates(service.ToolStates)
	s.mu.Lock()
	defer s.mu.Unlock()

	if service.ID == "" {
		service.ID = s.findServiceIDForUpdateLocked(service)
	}
	if service.ID == "" {
		service.ID = generateUniqueServiceID(s.cfg.MCP.Services, service.Name, service.Endpoint, service.Command)
	}
	if service.Name == "" {
		service.Name = service.ID
	}
	if err := validateService(service); err != nil {
		return err
	}

	now := time.Now()
	service.UpdatedAt = now

	updated := false
	for i := range s.cfg.MCP.Services {
		if s.cfg.MCP.Services[i].ID == service.ID {
			if service.AuthToken == "" {
				service.AuthToken = s.cfg.MCP.Services[i].AuthToken
			}
			if len(service.ToolStates) == 0 {
				service.ToolStates = cloneToolStates(s.cfg.MCP.Services[i].ToolStates)
			}
			s.cfg.MCP.Services[i] = service
			updated = true
			break
		}
	}
	if !updated {
		s.cfg.MCP.Services = append(s.cfg.MCP.Services, service)
	}

	return s.persistLocked()
}

func (s *Store) DeleteService(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("service id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := make([]Service, 0, len(s.cfg.MCP.Services))
	for _, svc := range s.cfg.MCP.Services {
		if svc.ID != id {
			next = append(next, svc)
		}
	}
	s.cfg.MCP.Services = next
	return s.persistLocked()
}

func (s *Store) SetEnabled(id string, enabled bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("service id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i := range s.cfg.MCP.Services {
		if s.cfg.MCP.Services[i].ID == id {
			s.cfg.MCP.Services[i].Enabled = enabled
			s.cfg.MCP.Services[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("service %q not found", id)
	}

	return s.persistLocked()
}

func (s *Store) SetServiceToolEnabled(serviceID, toolName string, enabled bool) error {
	serviceID = strings.TrimSpace(serviceID)
	toolName = strings.TrimSpace(toolName)
	if serviceID == "" {
		return fmt.Errorf("service id is required")
	}
	if toolName == "" {
		return fmt.Errorf("tool name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.cfg.MCP.Services {
		if s.cfg.MCP.Services[i].ID != serviceID {
			continue
		}

		now := time.Now()
		states := cloneToolStates(s.cfg.MCP.Services[i].ToolStates)
		idx := -1
		for j := range states {
			if states[j].Name == toolName {
				idx = j
				break
			}
		}

		if enabled {
			if idx >= 0 {
				states = append(states[:idx], states[idx+1:]...)
			}
		} else {
			if idx >= 0 {
				states[idx].Enabled = false
				states[idx].UpdatedAt = now
			} else {
				states = append(states, ServiceToolState{
					Name:      toolName,
					Enabled:   false,
					UpdatedAt: now,
				})
			}
		}

		s.cfg.MCP.Services[i].ToolStates = normalizeServiceToolStates(states)
		s.cfg.MCP.Services[i].UpdatedAt = now
		return s.persistLocked()
	}

	return fmt.Errorf("service %q not found", serviceID)
}

func (s *Store) IsServiceToolEnabled(serviceID, toolName string) bool {
	serviceID = strings.TrimSpace(serviceID)
	toolName = strings.TrimSpace(toolName)
	if serviceID == "" || toolName == "" {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, svc := range s.cfg.MCP.Services {
		if svc.ID == serviceID {
			return serviceToolEnabled(svc, toolName)
		}
	}
	return false
}
