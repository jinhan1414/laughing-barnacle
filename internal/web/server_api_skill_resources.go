package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"laughing-barnacle/internal/skills"
)

func (s *Server) handleAPISkillIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	skillID := strings.TrimSpace(r.URL.Query().Get("id"))
	if skillID == "" {
		writeSkillAPIError(w, http.StatusBadRequest, "query parameter id is required")
		return
	}
	index, err := s.skillStore.ReadEnabledSkillResourceIndex(skillID)
	if err != nil {
		writeSkillResourceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(index)
}

func (s *Server) handleAPISkillRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	skillID := strings.TrimSpace(r.URL.Query().Get("id"))
	if skillID == "" {
		writeSkillAPIError(w, http.StatusBadRequest, "query parameter id is required")
		return
	}
	resourcePath := strings.TrimSpace(r.URL.Query().Get("path"))
	content, err := s.skillStore.ReadEnabledSkillResource(skillID, resourcePath)
	if err != nil {
		writeSkillResourceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      skillID,
		"path":    strings.TrimSpace(defaultSkillReadPath(resourcePath)),
		"content": content,
	})
}

func defaultSkillReadPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "SKILL.md"
	}
	return strings.TrimSpace(path)
}

func writeSkillResourceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, skills.ErrSkillNotFound), errors.Is(err, skills.ErrSkillResourceNotFound):
		writeSkillAPIError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, skills.ErrInvalidSkillResourcePath):
		writeSkillAPIError(w, http.StatusBadRequest, err.Error())
	default:
		writeSkillAPIError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeSkillAPIError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}
