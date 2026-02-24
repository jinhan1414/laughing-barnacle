package web

import (
	"context"
	"fmt"
	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/skills"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleSettingsMCPSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "mcp", "", "请求参数解析失败")
		return
	}

	service := mcp.Service{
		ID:        "",
		Name:      strings.TrimSpace(r.FormValue("name")),
		Endpoint:  strings.TrimSpace(r.FormValue("endpoint")),
		Command:   strings.TrimSpace(r.FormValue("command")),
		Transport: strings.TrimSpace(r.FormValue("transport")),
		AuthToken: strings.TrimSpace(r.FormValue("auth_token")),
		Enabled:   r.FormValue("enabled") == "on",
	}
	args, err := parseJSONArgsList(strings.TrimSpace(r.FormValue("args_json")))
	if err != nil {
		s.redirectSettings(w, r, "mcp", "", err.Error())
		return
	}
	service.Args = args
	if err := s.mcpStore.UpsertService(service); err != nil {
		s.redirectSettings(w, r, "mcp", "", err.Error())
		return
	}
	s.mcpTools.InvalidateCache()
	s.redirectSettings(w, r, "mcp", "MCP 服务已保存", "")
}

func (s *Server) handleSettingsMCPDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "mcp", "", "请求参数解析失败")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if err := s.mcpStore.DeleteService(id); err != nil {
		s.redirectSettings(w, r, "mcp", "", err.Error())
		return
	}
	s.mcpTools.InvalidateCache()
	s.redirectSettings(w, r, "mcp", fmt.Sprintf("MCP 服务 %s 已删除", id), "")
}

func (s *Server) handleSettingsMCPToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "mcp", "", "请求参数解析失败")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	enable := r.FormValue("enabled") == "true"
	if err := s.mcpStore.SetEnabled(id, enable); err != nil {
		s.redirectSettings(w, r, "mcp", "", err.Error())
		return
	}
	s.mcpTools.InvalidateCache()
	if enable {
		s.redirectSettings(w, r, "mcp", fmt.Sprintf("MCP 服务 %s 已启用", id), "")
		return
	}
	s.redirectSettings(w, r, "mcp", fmt.Sprintf("MCP 服务 %s 已禁用", id), "")
}

func (s *Server) handleSettingsMCPToolToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "mcp", "", "请求参数解析失败")
		return
	}
	serviceID := strings.TrimSpace(r.FormValue("service_id"))
	toolName := strings.TrimSpace(r.FormValue("tool_name"))
	enable := r.FormValue("enabled") == "true"
	if err := s.mcpStore.SetServiceToolEnabled(serviceID, toolName, enable); err != nil {
		s.redirectSettings(w, r, "mcp", "", err.Error())
		return
	}
	s.mcpTools.InvalidateCache()
	if enable {
		s.redirectSettings(w, r, "mcp", fmt.Sprintf("工具 %s 已启用", toolName), "")
		return
	}
	s.redirectSettings(w, r, "mcp", fmt.Sprintf("工具 %s 已禁用", toolName), "")
}

func (s *Server) handleSettingsSkillInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "skills", "", "请求参数解析失败")
		return
	}

	rawURL := strings.TrimSpace(r.FormValue("skills_sh_url"))
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	installed, err := s.skillStore.InstallFromSkillsSH(ctx, rawURL)
	if err != nil {
		s.redirectSettings(w, r, "skills", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "skills", fmt.Sprintf("Skill 已安装：%s (%s)", installed.Name, installed.ID), "")
}

func (s *Server) handleSettingsSkillSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "skills", "", "请求参数解析失败")
		return
	}

	skill := skills.Skill{
		ID:          "",
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Prompt:      strings.TrimSpace(r.FormValue("prompt")),
		Enabled:     r.FormValue("enabled") == "on",
	}
	if err := s.skillStore.UpsertSkill(skill); err != nil {
		s.redirectSettings(w, r, "skills", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "skills", "Skill 已保存", "")
}

func (s *Server) handleSettingsSkillDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "skills", "", "请求参数解析失败")
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	if err := s.skillStore.DeleteSkill(id); err != nil {
		s.redirectSettings(w, r, "skills", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "skills", fmt.Sprintf("Skill %s 已删除", id), "")
}

func (s *Server) handleSettingsSkillToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "skills", "", "请求参数解析失败")
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	enable := r.FormValue("enabled") == "true"
	if err := s.skillStore.SetSkillEnabled(id, enable); err != nil {
		s.redirectSettings(w, r, "skills", "", err.Error())
		return
	}
	if enable {
		s.redirectSettings(w, r, "skills", fmt.Sprintf("Skill %s 已启用", id), "")
		return
	}
	s.redirectSettings(w, r, "skills", fmt.Sprintf("Skill %s 已禁用", id), "")
}
