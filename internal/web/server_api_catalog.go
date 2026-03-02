package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleAPIMCPServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	services := s.mcpStore.ListServices()
	items := make([]apiMCPService, 0, len(services))
	for _, svc := range services {
		items = append(items, apiMCPService{
			ID:         svc.ID,
			Name:       svc.Name,
			Transport:  strings.TrimSpace(svc.Transport),
			Endpoint:   strings.TrimSpace(svc.Endpoint),
			Command:    strings.TrimSpace(svc.Command),
			Args:       append([]string(nil), svc.Args...),
			HasEnv:     len(svc.Env) > 0,
			EnvKeys:    sortedMapKeys(svc.Env),
			HasHeaders: len(svc.Headers) > 0,
			HeaderKeys: sortedMapKeys(svc.Headers),
			Enabled:    svc.Enabled,
			UpdatedAt:  svc.UpdatedAt,
		})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"services": items})
}

func (s *Server) handleAPISkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	skillsList := s.skillStore.ListSkills()
	items := make([]apiSkill, 0, len(skillsList))
	for _, item := range skillsList {
		items = append(items, apiSkill{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			Source:      item.Source,
			Enabled:     item.Enabled,
			UpdatedAt:   item.UpdatedAt,
		})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"skills": items})
}

func (s *Server) handleAPISkillsCatalogSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "query parameter q is required",
		})
		return
	}

	limit := 8
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			limit = parsed
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	items, err := s.skillStore.SearchSkillsCatalog(ctx, query, limit)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"query":  query,
		"skills": items,
	})
}

func (s *Server) handleAPISchedules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	tasks := s.mcpStore.ListScheduledTasks()
	items := make([]apiScheduledTask, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, apiScheduledTask{
			ID:          task.ID,
			Name:        task.Name,
			Description: task.Description,
			Action:      task.Action,
			CronExpr:    task.CronExpr,
			Enabled:     task.Enabled,
			LastRunAt:   task.LastRunAt,
			LastStatus:  task.LastStatus,
			LastMessage: task.LastMessage,
			UpdatedAt:   task.UpdatedAt,
		})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schedules": items,
	})
}
