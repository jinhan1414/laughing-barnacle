package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

var serviceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type Service struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Endpoint  string    `json:"endpoint"`
	AuthToken string    `json:"auth_token,omitempty"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

type fileConfig struct {
	MCP struct {
		Services []Service `json:"services"`
	} `json:"mcp"`
}

type Store struct {
	path string
	mu   sync.RWMutex
	cfg  fileConfig
}

func NewStore(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("settings file path is required")
	}

	s := &Store{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) ListServices() []Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.MCP.Services)
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
			return svc, true
		}
	}
	return Service{}, false
}

func (s *Store) UpsertService(service Service) error {
	service.ID = strings.TrimSpace(service.ID)
	service.Name = strings.TrimSpace(service.Name)
	service.Endpoint = strings.TrimSpace(service.Endpoint)
	service.AuthToken = strings.TrimSpace(service.AuthToken)
	if service.Name == "" {
		service.Name = service.ID
	}
	if err := validateService(service); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	service.UpdatedAt = now

	updated := false
	for i := range s.cfg.MCP.Services {
		if s.cfg.MCP.Services[i].ID == service.ID {
			if service.AuthToken == "" {
				service.AuthToken = s.cfg.MCP.Services[i].AuthToken
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

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.cfg = fileConfig{}
			return s.persistLocked()
		}
		return fmt.Errorf("read settings file: %w", err)
	}

	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("decode settings file: %w", err)
	}
	for _, svc := range cfg.MCP.Services {
		if err := validateService(svc); err != nil {
			return fmt.Errorf("invalid mcp service %q: %w", svc.ID, err)
		}
	}

	s.cfg = cfg
	return nil
}

func (s *Store) persistLocked() error {
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}

	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp settings: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		return fmt.Errorf("rename settings file: %w", err)
	}
	return nil
}

func validateService(service Service) error {
	if service.ID == "" {
		return fmt.Errorf("service id is required")
	}
	if !serviceIDPattern.MatchString(service.ID) {
		return fmt.Errorf("service id must match [a-zA-Z0-9_-]+")
	}
	if service.Endpoint == "" {
		return fmt.Errorf("service endpoint is required")
	}
	if !strings.HasPrefix(service.Endpoint, "http://") && !strings.HasPrefix(service.Endpoint, "https://") {
		return fmt.Errorf("service endpoint must start with http:// or https://")
	}
	return nil
}
