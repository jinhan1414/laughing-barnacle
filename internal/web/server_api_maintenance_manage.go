package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/scheduler"
	"laughing-barnacle/internal/skills"
)

type apiMCPServiceSaveRequest struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Transport string            `json:"transport"`
	Endpoint  string            `json:"endpoint"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	AuthToken string            `json:"auth_token"`
	Env       map[string]string `json:"env"`
	Headers   map[string]string `json:"headers"`
	Enabled   bool              `json:"enabled"`
}

type apiMCPServiceToggleRequest struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

type apiMCPServiceDeleteRequest struct {
	ID string `json:"id"`
}

type apiSkillSaveRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	Enabled     bool   `json:"enabled"`
}

type apiSkillToggleRequest struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

type apiSkillDeleteRequest struct {
	ID string `json:"id"`
}

type apiSkillInstallRequest struct {
	SkillsSHURL string `json:"skills_sh_url"`
}

type apiScheduleSaveRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Action      string `json:"action"`
	CronExpr    string `json:"cron_expr"`
	Enabled     bool   `json:"enabled"`
}

type apiScheduleToggleRequest struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

type apiScheduleDeleteRequest struct {
	ID string `json:"id"`
}

type apiScheduleRunRequest struct {
	ID string `json:"id"`
}

func (s *Server) handleAPIMCPServiceSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req apiMCPServiceSaveRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIBadRequest(w, "invalid json body: "+err.Error())
		return
	}
	err := s.saveMCPService(mcp.Service{
		ID:        strings.TrimSpace(req.ID),
		Name:      strings.TrimSpace(req.Name),
		Transport: strings.TrimSpace(req.Transport),
		Endpoint:  strings.TrimSpace(req.Endpoint),
		Command:   strings.TrimSpace(req.Command),
		Args:      req.Args,
		AuthToken: strings.TrimSpace(req.AuthToken),
		Env:       req.Env,
		Headers:   req.Headers,
		Enabled:   req.Enabled,
	})
	if err != nil {
		writeAPIBadRequest(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAPIMCPServiceToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req apiMCPServiceToggleRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIBadRequest(w, "invalid json body: "+err.Error())
		return
	}
	if err := s.toggleMCPService(req.ID, req.Enabled); err != nil {
		writeAPIBadRequest(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAPIMCPServiceDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req apiMCPServiceDeleteRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIBadRequest(w, "invalid json body: "+err.Error())
		return
	}
	if err := s.deleteMCPService(req.ID); err != nil {
		writeAPIBadRequest(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAPISkillSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req apiSkillSaveRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIBadRequest(w, "invalid json body: "+err.Error())
		return
	}
	err := s.saveSkill(skills.Skill{
		ID:          strings.TrimSpace(req.ID),
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		Prompt:      strings.TrimSpace(req.Prompt),
		Enabled:     req.Enabled,
	})
	if err != nil {
		writeAPIBadRequest(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAPISkillToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req apiSkillToggleRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIBadRequest(w, "invalid json body: "+err.Error())
		return
	}
	if err := s.toggleSkill(req.ID, req.Enabled); err != nil {
		writeAPIBadRequest(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAPISkillDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req apiSkillDeleteRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIBadRequest(w, "invalid json body: "+err.Error())
		return
	}
	if err := s.deleteSkill(req.ID); err != nil {
		writeAPIBadRequest(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAPISkillInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req apiSkillInstallRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIBadRequest(w, "invalid json body: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	installed, err := s.installSkill(ctx, req.SkillsSHURL)
	if err != nil {
		writeAPIBadRequest(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": installed.ID, "name": installed.Name})
}

func (s *Server) handleAPIScheduleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req apiScheduleSaveRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIBadRequest(w, "invalid json body: "+err.Error())
		return
	}
	err := s.saveSchedule(scheduler.Task{
		ID:          strings.TrimSpace(req.ID),
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		Action:      strings.TrimSpace(req.Action),
		CronExpr:    strings.TrimSpace(req.CronExpr),
		Enabled:     req.Enabled,
	})
	if err != nil {
		writeAPIBadRequest(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAPIScheduleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req apiScheduleToggleRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIBadRequest(w, "invalid json body: "+err.Error())
		return
	}
	if err := s.toggleSchedule(req.ID, req.Enabled); err != nil {
		writeAPIBadRequest(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAPIScheduleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req apiScheduleDeleteRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIBadRequest(w, "invalid json body: "+err.Error())
		return
	}
	result, err := s.deleteSchedule(req.ID)
	if err != nil {
		writeAPIBadRequest(w, err.Error())
		return
	}
	payload := map[string]any{"ok": true, "message": result.Message}
	if strings.TrimSpace(result.Warning) != "" {
		payload["warning"] = result.Warning
	}
	writeJSON(w, payload)
}

func (s *Server) handleAPIScheduleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req apiScheduleRunRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIBadRequest(w, "invalid json body: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := s.runScheduleNow(ctx, req.ID); err != nil {
		writeAPIBadRequest(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": fmt.Sprintf("定时任务 %s 已立即执行", strings.TrimSpace(req.ID))})
}

func decodeJSONBody(r *http.Request, dest any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("json body must contain a single object")
	}
	return nil
}

func writeAPIBadRequest(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusBadRequest)
	writeJSON(w, map[string]any{"error": strings.TrimSpace(message)})
}
