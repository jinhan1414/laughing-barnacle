package web

import (
	"encoding/json"
	"laughing-barnacle/internal/mcp"
	"net/http"
	"strings"
)

func (s *Server) handleAPIA2AAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	agents := s.mcpStore.ListA2AAgents()
	items := make([]apiA2AAgent, 0, len(agents))
	for _, item := range agents {
		items = append(items, apiA2AAgent{
			ID:           item.ID,
			Name:         item.Name,
			Description:  item.Description,
			Endpoint:     item.Endpoint,
			AgentCardURL: item.AgentCardURL,
			ProtocolVersion: item.ProtocolVersion,
			Skills:       append([]mcp.A2ASkill(nil), item.Skills...),
			Enabled:      item.Enabled,
			UpdatedAt:    item.UpdatedAt,
			HasAuthToken: strings.TrimSpace(item.AuthToken) != "",
		})
	}
	writeJSON(w, map[string]any{"agents": items})
}

func (s *Server) handleAPIA2AAgentRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": "query parameter id is required"})
		return
	}
	item, ok := s.mcpStore.GetA2AAgent(agentID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]any{"error": "agent not found"})
		return
	}
	writeJSON(w, map[string]any{
		"agent": apiA2AAgent{
			ID:           item.ID,
			Name:         item.Name,
			Description:  item.Description,
			Endpoint:     item.Endpoint,
			AgentCardURL: item.AgentCardURL,
			ProtocolVersion: item.ProtocolVersion,
			Skills:       append([]mcp.A2ASkill(nil), item.Skills...),
			Enabled:      item.Enabled,
			UpdatedAt:    item.UpdatedAt,
			HasAuthToken: strings.TrimSpace(item.AuthToken) != "",
		},
	})
}

func writeJSON(w http.ResponseWriter, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}
