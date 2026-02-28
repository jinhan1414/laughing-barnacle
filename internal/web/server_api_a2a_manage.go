package web

import (
	"encoding/json"
	"laughing-barnacle/internal/mcp"
	"net/http"
	"strings"
)

type apiA2ASaveRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Endpoint     string `json:"endpoint"`
	AgentCardURL string `json:"agent_card_url"`
	ProtocolVersion string `json:"protocol_version"`
	Skills       []mcp.A2ASkill `json:"skills"`
	AuthToken    string `json:"auth_token"`
	Enabled      bool   `json:"enabled"`
}

type apiA2AToggleRequest struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

type apiA2ADeleteRequest struct {
	ID string `json:"id"`
}

func (s *Server) handleAPIA2AAgentSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req apiA2ASaveRequest
	if err := decodeA2AManageRequest(r, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": "invalid json body: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.AgentCardURL) != "" {
		discovered, err := discoverA2AAgentFromCard(r.Context(), req.AgentCardURL, req.AuthToken)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		applyDiscoveredA2AAgentFields(&req, discovered)
	}

	item := mcp.A2AAgent{
		ID:           strings.TrimSpace(req.ID),
		Name:         strings.TrimSpace(req.Name),
		Description:  strings.TrimSpace(req.Description),
		Endpoint:     strings.TrimSpace(req.Endpoint),
		AgentCardURL: strings.TrimSpace(req.AgentCardURL),
		ProtocolVersion: strings.TrimSpace(req.ProtocolVersion),
		Skills:       append([]mcp.A2ASkill(nil), req.Skills...),
		AuthToken:    strings.TrimSpace(req.AuthToken),
		Enabled:      req.Enabled,
	}
	if err := s.mcpStore.UpsertA2AAgent(item); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAPIA2AAgentToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req apiA2AToggleRequest
	if err := decodeA2AManageRequest(r, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": "invalid json body: " + err.Error()})
		return
	}
	if err := s.mcpStore.SetA2AAgentEnabled(strings.TrimSpace(req.ID), req.Enabled); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAPIA2AAgentDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req apiA2ADeleteRequest
	if err := decodeA2AManageRequest(r, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": "invalid json body: " + err.Error()})
		return
	}
	if err := s.mcpStore.DeleteA2AAgent(strings.TrimSpace(req.ID)); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

func decodeA2AManageRequest(r *http.Request, dest any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}
