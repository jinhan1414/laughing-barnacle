package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/scheduler"
)

func (s *Server) handleSettingsScheduleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "schedules", "", "请求参数解析失败")
		return
	}

	task := scheduler.Task{
		ID:          strings.TrimSpace(r.FormValue("id")),
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Action:      strings.TrimSpace(r.FormValue("action")),
		CronExpr:    strings.TrimSpace(r.FormValue("cron_expr")),
		Enabled:     parseEnabledFormValue(r.FormValue("enabled")),
	}
	if err := s.saveSchedule(task); err != nil {
		s.redirectSettings(w, r, "schedules", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "schedules", "定时任务已保存", "")
}

func (s *Server) handleSettingsScheduleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "schedules", "", "请求参数解析失败")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	result, err := s.deleteSchedule(id)
	if err != nil {
		s.redirectSettings(w, r, "schedules", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "schedules", result.Message, result.Warning)
}

func (s *Server) handleSettingsScheduleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "schedules", "", "请求参数解析失败")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	enable := parseEnabledFormValue(r.FormValue("enabled"))
	if err := s.toggleSchedule(id, enable); err != nil {
		s.redirectSettings(w, r, "schedules", "", err.Error())
		return
	}
	if enable {
		s.redirectSettings(w, r, "schedules", fmt.Sprintf("定时任务 %s 已启用", id), "")
		return
	}
	s.redirectSettings(w, r, "schedules", fmt.Sprintf("定时任务 %s 已禁用", id), "")
}

func (s *Server) handleSettingsScheduleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "schedules", "", "请求参数解析失败")
		return
	}
	taskID := strings.TrimSpace(r.FormValue("id"))
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := s.runScheduleNow(ctx, taskID); err != nil {
		s.redirectSettings(w, r, "schedules", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "schedules", fmt.Sprintf("定时任务 %s 已立即执行", taskID), "")
}

func (s *Server) handleSettingsMemoryInboxReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.memoryStore == nil {
		s.redirectSettings(w, r, "memory", "", "memory store unavailable")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "memory", "", "请求参数解析失败")
		return
	}
	path := strings.TrimSpace(r.FormValue("path"))
	action := strings.TrimSpace(r.FormValue("action"))
	target, err := s.memoryStore.ReviewInboxCandidate(path, action)
	if err != nil {
		s.redirectSettings(w, r, "memory", "", err.Error())
		return
	}
	if strings.EqualFold(action, "reject") {
		s.redirectSettings(w, r, "memory", "候选记忆已拒绝并移入回收区："+target, "")
		return
	}
	s.redirectSettings(w, r, "memory", "候选记忆已确认并写入："+target, "")
}

func (s *Server) handleSettingsMemoryMaintenanceRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_ = r.ParseForm()
	redirectSection := "memory"
	if candidate := strings.TrimSpace(r.FormValue("redirect_section")); candidate == "schedules" || candidate == "memory" {
		redirectSection = candidate
	}
	if s.memoryStore == nil {
		s.redirectSettings(w, r, redirectSection, "", "memory store unavailable")
		return
	}
	report, err := s.memoryStore.RunMaintenance(time.Now().UTC(), 0, 0)
	if err != nil {
		s.redirectSettings(w, r, redirectSection, "", "维护任务执行失败："+err.Error())
		return
	}
	s.redirectSettings(
		w,
		r,
		redirectSection,
		fmt.Sprintf("维护任务完成：retried=%d, cleaned=%d, repaired=%d", report.RetriedSegments, report.RemovedTrashNodes, report.RepairedChildren),
		"",
	)
}

func (s *Server) handleSettingsLLMPromptsSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "llm", "", "请求参数解析失败")
		return
	}

	cfg := mcp.AgentPromptConfig{
		SystemPrompt:            strings.TrimSpace(r.FormValue("system_prompt")),
		CompressionSystemPrompt: strings.TrimSpace(r.FormValue("compression_system_prompt")),
	}
	if err := s.mcpStore.UpsertAgentPromptConfig(cfg); err != nil {
		s.redirectSettings(w, r, "llm", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "llm", "系统提示词已更新", "")
}

func (s *Server) handleSettingsLLMPromptsReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := s.mcpStore.ResetAgentPromptConfig(); err != nil {
		s.redirectSettings(w, r, "llm", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "llm", "已重置为内置默认提示词", "")
}
