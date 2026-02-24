package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"laughing-barnacle/internal/memory"
)

func (s *Server) handleAPIMemoryIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.memoryStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "memory store unavailable"})
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		path = "/"
	}
	items, err := s.memoryStore.ListIndex(path)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, memory.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	respItems := make([]apiMemoryIndexItem, 0, len(items))
	for _, item := range items {
		respItems = append(respItems, apiMemoryIndexItem{
			Path:      item.Path,
			Title:     item.Title,
			Type:      item.Type,
			Summary:   item.Summary,
			Revision:  item.Revision,
			UpdatedAt: item.UpdatedAt,
		})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path":  path,
		"items": respItems,
	})
}

func (s *Server) handleAPIMemoryRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.memoryStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "memory store unavailable"})
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "query parameter path is required"})
		return
	}
	node, err := s.memoryStore.ReadNode(path)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, memory.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(node)
}

func (s *Server) handleAPIMemorySection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.memoryStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "memory store unavailable"})
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	sectionID := strings.TrimSpace(r.URL.Query().Get("section_id"))
	if path == "" || sectionID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "query parameter path and section_id are required"})
		return
	}
	section, err := s.memoryStore.ReadSection(path, sectionID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, memory.ErrNodeNotFound) || errors.Is(err, memory.ErrSectionNotFound) {
			status = http.StatusNotFound
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path":    path,
		"section": section,
	})
}

func (s *Server) handleAPIMemoryUpsert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.memoryStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "memory store unavailable"})
		return
	}
	var req struct {
		Mode             string           `json:"mode"`
		Path             string           `json:"path"`
		Title            string           `json:"title"`
		Type             memory.NodeType  `json:"type"`
		SchemaKind       string           `json:"schema_kind"`
		SchemaVersion    int              `json:"schema_version"`
		Tags             []string         `json:"tags"`
		Source           string           `json:"source"`
		Confidence       float64          `json:"confidence"`
		Summary          string           `json:"summary"`
		Facts            []string         `json:"facts"`
		Sections         []memory.Section `json:"sections"`
		Refs             []memory.Ref     `json:"refs"`
		ExpectedRevision int64            `json:"expected_revision"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid request body"})
		return
	}
	node, err := s.memoryStore.UpsertNode(memory.UpsertRequest{
		Mode:             req.Mode,
		Path:             req.Path,
		Title:            req.Title,
		Type:             req.Type,
		SchemaKind:       req.SchemaKind,
		SchemaVersion:    req.SchemaVersion,
		Tags:             req.Tags,
		Source:           req.Source,
		Confidence:       req.Confidence,
		Summary:          req.Summary,
		Facts:            req.Facts,
		Sections:         req.Sections,
		Refs:             req.Refs,
		ExpectedRevision: req.ExpectedRevision,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, memory.ErrRevisionConflict) {
			status = http.StatusConflict
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"node": node})
}

func (s *Server) handleAPIMemoryMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.memoryStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "memory store unavailable"})
		return
	}
	var req struct {
		FromPath         string `json:"from_path"`
		ToPath           string `json:"to_path"`
		ExpectedRevision int64  `json:"expected_revision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid request body"})
		return
	}
	if err := s.memoryStore.MoveNode(req.FromPath, req.ToPath, req.ExpectedRevision); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, memory.ErrNodeNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, memory.ErrRevisionConflict) {
			status = http.StatusConflict
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleAPIMemoryDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.memoryStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "memory store unavailable"})
		return
	}
	var req struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid request body"})
		return
	}
	soft := strings.ToLower(strings.TrimSpace(req.Mode)) != "hard"
	movedTo, err := s.memoryStore.DeleteNode(req.Path, soft)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, memory.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "moved_to": movedTo})
}
