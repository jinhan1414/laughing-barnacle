package mcp

import "strings"

func generateUniqueA2AAgentID(existing []A2AAgent, name, endpoint, cardURL string) string {
	used := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		used[item.ID] = struct{}{}
	}
	return generateUniqueID(used, []string{name, endpoint, cardURL}, "a2a-agent")
}

func (s *Store) findA2AAgentIDForUpdateLocked(agent A2AAgent) string {
	endpoint := strings.TrimSpace(agent.Endpoint)
	cardURL := strings.TrimSpace(agent.AgentCardURL)

	if endpoint != "" {
		for _, item := range s.cfg.A2A.Agents {
			if strings.EqualFold(strings.TrimSpace(item.Endpoint), endpoint) {
				return item.ID
			}
		}
	}
	if cardURL != "" {
		for _, item := range s.cfg.A2A.Agents {
			if strings.EqualFold(strings.TrimSpace(item.AgentCardURL), cardURL) {
				return item.ID
			}
		}
	}
	return ""
}
