package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

var serviceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

const (
	ServiceTransportStreamableHTTP = "streamable_http"
	ServiceTransportSSE            = "sse"
)

type Service struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Endpoint   string             `json:"endpoint"`
	Transport  string             `json:"transport,omitempty"`
	AuthToken  string             `json:"auth_token,omitempty"`
	Enabled    bool               `json:"enabled"`
	ToolStates []ServiceToolState `json:"tool_states,omitempty"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

type ServiceToolState struct {
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type Skill struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Prompt    string    `json:"prompt"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AgentPromptConfig struct {
	SystemPrompt            string    `json:"system_prompt"`
	CompressionSystemPrompt string    `json:"compression_system_prompt"`
	UpdatedAt               time.Time `json:"updated_at,omitempty"`
}

type fileConfig struct {
	MCP struct {
		Services []Service `json:"services"`
	} `json:"mcp"`
	Skills struct {
		Items []Skill `json:"items"`
	} `json:"skills"`
	Agent struct {
		Prompts AgentPromptConfig `json:"prompts"`
	} `json:"agent"`
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
	service.Transport = normalizeServiceTransport(service.Transport)
	service.AuthToken = strings.TrimSpace(service.AuthToken)
	service.ToolStates = normalizeServiceToolStates(service.ToolStates)
	s.mu.Lock()
	defer s.mu.Unlock()

	if service.ID == "" {
		service.ID = s.findServiceIDForUpdateLocked(service)
	}
	if service.ID == "" {
		service.ID = generateUniqueServiceID(s.cfg.MCP.Services, service.Name, service.Endpoint)
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

func (s *Store) ListSkills() []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.Skills.Items)
}

func (s *Store) ListEnabledSkillPrompts() []string {
	skills := s.ListSkills()
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		prompt := strings.TrimSpace(skill.Prompt)
		if prompt == "" {
			continue
		}
		out = append(out, prompt)
	}
	return out
}

func (s *Store) UpsertSkill(skill Skill) error {
	skill.ID = strings.TrimSpace(skill.ID)
	skill.Name = strings.TrimSpace(skill.Name)
	skill.Prompt = strings.TrimSpace(skill.Prompt)
	s.mu.Lock()
	defer s.mu.Unlock()

	if skill.ID == "" {
		skill.ID = s.findSkillIDForUpdateLocked(skill)
	}
	if skill.ID == "" {
		skill.ID = generateUniqueSkillID(s.cfg.Skills.Items, skill.Name, skill.Prompt)
	}
	if skill.Name == "" {
		skill.Name = skill.ID
	}
	if err := validateSkill(skill); err != nil {
		return err
	}

	skill.UpdatedAt = time.Now()
	updated := false
	for i := range s.cfg.Skills.Items {
		if s.cfg.Skills.Items[i].ID == skill.ID {
			s.cfg.Skills.Items[i] = skill
			updated = true
			break
		}
	}
	if !updated {
		s.cfg.Skills.Items = append(s.cfg.Skills.Items, skill)
	}

	return s.persistLocked()
}

func (s *Store) DeleteSkill(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skill id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := make([]Skill, 0, len(s.cfg.Skills.Items))
	for _, skill := range s.cfg.Skills.Items {
		if skill.ID != id {
			next = append(next, skill)
		}
	}
	s.cfg.Skills.Items = next
	return s.persistLocked()
}

func (s *Store) SetSkillEnabled(id string, enabled bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skill id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i := range s.cfg.Skills.Items {
		if s.cfg.Skills.Items[i].ID == id {
			s.cfg.Skills.Items[i].Enabled = enabled
			s.cfg.Skills.Items[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("skill %q not found", id)
	}
	return s.persistLocked()
}

func (s *Store) GetAgentPromptConfig() AgentPromptConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg := s.cfg.Agent.Prompts
	cfg.SystemPrompt = strings.TrimSpace(cfg.SystemPrompt)
	cfg.CompressionSystemPrompt = strings.TrimSpace(cfg.CompressionSystemPrompt)
	return cfg
}

func (s *Store) GetSystemPrompt() string {
	return s.GetAgentPromptConfig().SystemPrompt
}

func (s *Store) GetCompressionSystemPrompt() string {
	return s.GetAgentPromptConfig().CompressionSystemPrompt
}

func (s *Store) UpsertAgentPromptConfig(cfg AgentPromptConfig) error {
	cfg.SystemPrompt = strings.TrimSpace(cfg.SystemPrompt)
	cfg.CompressionSystemPrompt = strings.TrimSpace(cfg.CompressionSystemPrompt)

	if err := validateAgentPromptConfig(cfg); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg.UpdatedAt = time.Now()
	s.cfg.Agent.Prompts = cfg
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
	for i, svc := range cfg.MCP.Services {
		svc.Transport = normalizeServiceTransport(svc.Transport)
		svc.ToolStates = normalizeServiceToolStates(svc.ToolStates)
		if err := validateService(svc); err != nil {
			return fmt.Errorf("invalid mcp service %q: %w", svc.ID, err)
		}
		cfg.MCP.Services[i] = svc
	}
	for _, skill := range cfg.Skills.Items {
		if err := validateSkill(skill); err != nil {
			return fmt.Errorf("invalid skill %q: %w", skill.ID, err)
		}
	}
	if err := validateAgentPromptConfig(cfg.Agent.Prompts); err != nil {
		return fmt.Errorf("invalid agent prompts: %w", err)
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
	if service.Transport != ServiceTransportStreamableHTTP && service.Transport != ServiceTransportSSE {
		return fmt.Errorf("service transport must be streamable_http or sse")
	}
	for _, state := range service.ToolStates {
		if strings.TrimSpace(state.Name) == "" {
			return fmt.Errorf("service tool state name is required")
		}
	}
	return nil
}

func validateSkill(skill Skill) error {
	if skill.ID == "" {
		return fmt.Errorf("skill id is required")
	}
	if !serviceIDPattern.MatchString(skill.ID) {
		return fmt.Errorf("skill id must match [a-zA-Z0-9_-]+")
	}
	if strings.TrimSpace(skill.Name) == "" {
		return fmt.Errorf("skill name is required")
	}
	if strings.TrimSpace(skill.Prompt) == "" {
		return fmt.Errorf("skill prompt is required")
	}
	return nil
}

func validateAgentPromptConfig(cfg AgentPromptConfig) error {
	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	compressionPrompt := strings.TrimSpace(cfg.CompressionSystemPrompt)
	if systemPrompt == "" && compressionPrompt == "" {
		return nil
	}
	if systemPrompt == "" {
		return fmt.Errorf("agent system prompt is required")
	}
	if compressionPrompt == "" {
		return fmt.Errorf("agent compression system prompt is required")
	}
	return nil
}

func generateUniqueServiceID(existing []Service, name, endpoint string) string {
	used := make(map[string]struct{}, len(existing))
	for _, svc := range existing {
		used[svc.ID] = struct{}{}
	}
	return generateUniqueID(used, []string{name, endpoint}, "service")
}

func generateUniqueSkillID(existing []Skill, name, prompt string) string {
	used := make(map[string]struct{}, len(existing))
	for _, skill := range existing {
		used[skill.ID] = struct{}{}
	}
	return generateUniqueID(used, []string{name, prompt}, "skill")
}

func generateUniqueID(used map[string]struct{}, candidates []string, fallback string) string {
	base := ""
	for _, candidate := range candidates {
		base = sanitizeIdentifier(candidate)
		if base != "" {
			break
		}
	}
	if base == "" {
		base = fallback
	}
	if _, exists := used[base]; !exists {
		return base
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s-%d", base, i)
		if _, exists := used[next]; !exists {
			return next
		}
	}
}

func sanitizeIdentifier(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteRune('-')
			lastDash = true
		default:
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteRune('-')
			lastDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}

func normalizeServiceTransport(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "", "streamablehttp", "streamable_http", "streamable-http":
		return ServiceTransportStreamableHTTP
	case "sse":
		return ServiceTransportSSE
	default:
		return normalized
	}
}

func (s *Store) findServiceIDForUpdateLocked(service Service) string {
	endpoint := strings.TrimSpace(service.Endpoint)
	if endpoint != "" {
		for _, existing := range s.cfg.MCP.Services {
			if strings.TrimSpace(existing.Endpoint) == endpoint {
				return existing.ID
			}
		}
	}
	return ""
}

func (s *Store) findSkillIDForUpdateLocked(skill Skill) string {
	name := strings.TrimSpace(skill.Name)
	if name == "" {
		return ""
	}

	matchedID := ""
	for _, existing := range s.cfg.Skills.Items {
		if !strings.EqualFold(strings.TrimSpace(existing.Name), name) {
			continue
		}
		if matchedID != "" {
			return ""
		}
		matchedID = existing.ID
	}
	return matchedID
}

func cloneServices(in []Service) []Service {
	out := make([]Service, len(in))
	for i := range in {
		out[i] = cloneService(in[i])
	}
	return out
}

func cloneService(in Service) Service {
	out := in
	out.ToolStates = cloneToolStates(in.ToolStates)
	return out
}

func cloneToolStates(in []ServiceToolState) []ServiceToolState {
	if len(in) == 0 {
		return nil
	}
	out := make([]ServiceToolState, len(in))
	copy(out, in)
	return out
}

func normalizeServiceToolStates(states []ServiceToolState) []ServiceToolState {
	if len(states) == 0 {
		return nil
	}

	byName := make(map[string]ServiceToolState, len(states))
	for _, state := range states {
		name := strings.TrimSpace(state.Name)
		if name == "" {
			continue
		}
		// Default is enabled; only store explicit non-default states.
		if state.Enabled {
			delete(byName, name)
			continue
		}
		state.Name = name
		state.Enabled = false
		byName[name] = state
	}
	if len(byName) == 0 {
		return nil
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]ServiceToolState, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out
}

func serviceToolEnabled(service Service, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	for _, state := range service.ToolStates {
		if strings.TrimSpace(state.Name) == toolName {
			return state.Enabled
		}
	}
	return true
}
