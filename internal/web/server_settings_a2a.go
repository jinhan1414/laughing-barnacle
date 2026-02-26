package web

import (
	"fmt"
	"laughing-barnacle/internal/mcp"
	"net/http"
	"strings"
)

func (s *Server) handleSettingsA2ASave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "a2a", "", "请求参数解析失败")
		return
	}

	item := mcp.A2AAgent{
		ID:           strings.TrimSpace(r.FormValue("id")),
		Name:         strings.TrimSpace(r.FormValue("name")),
		Description:  strings.TrimSpace(r.FormValue("description")),
		Endpoint:     strings.TrimSpace(r.FormValue("endpoint")),
		AgentCardURL: strings.TrimSpace(r.FormValue("agent_card_url")),
		AuthToken:    strings.TrimSpace(r.FormValue("auth_token")),
		Enabled:      parseEnabledFormValue(r.FormValue("enabled")),
	}
	if err := s.mcpStore.UpsertA2AAgent(item); err != nil {
		s.redirectSettings(w, r, "a2a", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "a2a", "A2A Agent 已保存", "")
}

func (s *Server) handleSettingsA2ADelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "a2a", "", "请求参数解析失败")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if err := s.mcpStore.DeleteA2AAgent(id); err != nil {
		s.redirectSettings(w, r, "a2a", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "a2a", fmt.Sprintf("A2A Agent %s 已删除", id), "")
}

func (s *Server) handleSettingsA2AToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "a2a", "", "请求参数解析失败")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	enabled := parseEnabledFormValue(r.FormValue("enabled"))
	if err := s.mcpStore.SetA2AAgentEnabled(id, enabled); err != nil {
		s.redirectSettings(w, r, "a2a", "", err.Error())
		return
	}
	if enabled {
		s.redirectSettings(w, r, "a2a", fmt.Sprintf("A2A Agent %s 已启用", id), "")
		return
	}
	s.redirectSettings(w, r, "a2a", fmt.Sprintf("A2A Agent %s 已禁用", id), "")
}

func parseEnabledFormValue(raw string) bool {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	return normalized == "on" || normalized == "true" || normalized == "1"
}
