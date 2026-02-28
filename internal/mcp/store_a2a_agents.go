package mcp

import (
	"fmt"
	"strings"
	"time"
)

func (s *Store) ListA2AAgents() []A2AAgent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneA2AAgents(s.cfg.A2A.Agents)
}

func (s *Store) ListEnabledA2AAgents() []A2AAgent {
	all := s.ListA2AAgents()
	out := make([]A2AAgent, 0, len(all))
	for _, item := range all {
		if item.Enabled {
			out = append(out, item)
		}
	}
	return out
}

func (s *Store) GetA2AAgent(id string) (A2AAgent, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return A2AAgent{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.cfg.A2A.Agents {
		if item.ID == id {
			return cloneA2AAgent(item), true
		}
	}
	return A2AAgent{}, false
}

func (s *Store) UpsertA2AAgent(agent A2AAgent) error {
	agent.ID = strings.TrimSpace(agent.ID)
	agent.Name = strings.TrimSpace(agent.Name)
	agent.Description = normalizeSkillDescription(agent.Description, agent.Name, agent.Endpoint)
	agent.Endpoint = strings.TrimSpace(agent.Endpoint)
	agent.AgentCardURL = strings.TrimSpace(agent.AgentCardURL)
	agent.ProtocolVersion = strings.TrimSpace(agent.ProtocolVersion)
	agent.Skills = normalizeA2ASkills(agent.Skills)
	agent.AuthToken = strings.TrimSpace(agent.AuthToken)

	s.mu.Lock()
	defer s.mu.Unlock()

	if agent.ID == "" {
		agent.ID = s.findA2AAgentIDForUpdateLocked(agent)
	}
	if agent.ID == "" {
		agent.ID = generateUniqueA2AAgentID(s.cfg.A2A.Agents, agent.Name, agent.Endpoint, agent.AgentCardURL)
	}
	if agent.Name == "" {
		agent.Name = agent.ID
	}
	if err := validateA2AAgent(agent); err != nil {
		return err
	}

	now := time.Now()
	agent.UpdatedAt = now

	updated := false
	for i := range s.cfg.A2A.Agents {
		if s.cfg.A2A.Agents[i].ID != agent.ID {
			continue
		}
		if agent.AuthToken == "" {
			agent.AuthToken = s.cfg.A2A.Agents[i].AuthToken
		}
		if len(agent.Skills) == 0 {
			agent.Skills = cloneA2ASkills(s.cfg.A2A.Agents[i].Skills)
		}
		s.cfg.A2A.Agents[i] = agent
		updated = true
		break
	}
	if !updated {
		s.cfg.A2A.Agents = append(s.cfg.A2A.Agents, agent)
	}
	return s.persistLocked()
}

func (s *Store) DeleteA2AAgent(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("agent id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := make([]A2AAgent, 0, len(s.cfg.A2A.Agents))
	for _, item := range s.cfg.A2A.Agents {
		if item.ID != id {
			next = append(next, item)
		}
	}
	s.cfg.A2A.Agents = next
	return s.persistLocked()
}

func (s *Store) SetA2AAgentEnabled(id string, enabled bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("agent id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.cfg.A2A.Agents {
		if s.cfg.A2A.Agents[i].ID != id {
			continue
		}
		s.cfg.A2A.Agents[i].Enabled = enabled
		s.cfg.A2A.Agents[i].UpdatedAt = time.Now()
		return s.persistLocked()
	}
	return fmt.Errorf("agent %q not found", id)
}

func cloneA2AAgents(in []A2AAgent) []A2AAgent {
	if len(in) == 0 {
		return nil
	}
	out := make([]A2AAgent, 0, len(in))
	for _, item := range in {
		out = append(out, cloneA2AAgent(item))
	}
	return out
}

func cloneA2AAgent(in A2AAgent) A2AAgent {
	out := in
	out.Skills = cloneA2ASkills(in.Skills)
	return out
}

func normalizeA2ASkills(in []A2ASkill) []A2ASkill {
	if len(in) == 0 {
		return nil
	}
	out := make([]A2ASkill, 0, len(in))
	for _, item := range in {
		id := strings.TrimSpace(item.ID)
		name := strings.TrimSpace(item.Name)
		desc := strings.TrimSpace(item.Description)
		if id == "" && name == "" && desc == "" {
			continue
		}
		if id == "" {
			id = name
		}
		if name == "" {
			name = id
		}
		out = append(out, A2ASkill{
			ID:          id,
			Name:        name,
			Description: desc,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneA2ASkills(in []A2ASkill) []A2ASkill {
	if len(in) == 0 {
		return nil
	}
	out := make([]A2ASkill, len(in))
	copy(out, in)
	return out
}
