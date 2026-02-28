package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"laughing-barnacle/internal/conversation"
)

type apiChatArchiveIndexItem struct {
	ArchiveID string                      `json:"archive_id"`
	Title     string                      `json:"title"`
	Digest    string                      `json:"digest"`
	CreatedAt string                      `json:"created_at,omitempty"`
	Sections  []apiChatArchiveSectionMeta `json:"sections,omitempty"`
}

type apiChatArchiveSectionMeta struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Digest string `json:"digest"`
}

type apiChatArchiveSectionResponse struct {
	ArchiveID        string                              `json:"archive_id"`
	ID               string                              `json:"id"`
	Title            string                              `json:"title"`
	Digest           string                              `json:"digest"`
	Content          string                              `json:"content"`
	Messages         []conversation.ArchiveReplayMessage `json:"messages,omitempty"`
	LegacyIncomplete bool                                `json:"legacy_incomplete,omitempty"`
	Notice           string                              `json:"notice,omitempty"`
}

func (s *Server) handleAPIChatArchiveIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.convStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	ids := s.convStore.ListSummaryArchiveIDs()
	items := make([]apiChatArchiveIndexItem, 0, len(ids))
	for _, archiveID := range ids {
		index, err := s.convStore.ReadArchiveIndex(archiveID)
		if err != nil {
			continue
		}
		items = append(items, toAPIChatArchiveIndexItem(index))
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"archives": items,
	})
}

func (s *Server) handleAPIChatArchiveSection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.convStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	archiveID := strings.TrimSpace(r.URL.Query().Get("archive_id"))
	sectionID := strings.TrimSpace(r.URL.Query().Get("section_id"))
	if archiveID == "" || sectionID == "" {
		writeChatArchiveError(w, http.StatusBadRequest, "archive_id and section_id are required")
		return
	}

	section, err := s.convStore.ReadArchiveSection(archiveID, sectionID)
	if err != nil {
		if errors.Is(err, conversation.ErrArchiveNotFound) || errors.Is(err, conversation.ErrArchiveSectionNotFound) {
			writeChatArchiveError(w, http.StatusNotFound, err.Error())
			return
		}
		writeChatArchiveError(w, http.StatusInternalServerError, "read archive section failed")
		return
	}

	resp := apiChatArchiveSectionResponse{
		ArchiveID:        archiveID,
		ID:               section.ID,
		Title:            section.Title,
		Digest:           section.Digest,
		Content:          section.Content,
		Messages:         section.Messages,
		LegacyIncomplete: section.LegacyIncomplete,
	}
	if section.LegacyIncomplete {
		resp.Notice = "该归档为旧格式，内容可能不完整。"
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

func toAPIChatArchiveIndexItem(index conversation.ArchiveIndex) apiChatArchiveIndexItem {
	sections := make([]apiChatArchiveSectionMeta, 0, len(index.Sections))
	for _, sec := range index.Sections {
		sections = append(sections, apiChatArchiveSectionMeta{
			ID:     sec.ID,
			Title:  sec.Title,
			Digest: sec.Digest,
		})
	}
	item := apiChatArchiveIndexItem{
		ArchiveID: index.ArchiveID,
		Title:     index.Title,
		Digest:    index.Digest,
		Sections:  sections,
	}
	if !index.CreatedAt.IsZero() {
		item.CreatedAt = index.CreatedAt.Local().Format("2006-01-02 15:04:05")
	}
	return item
}

func writeChatArchiveError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": strings.TrimSpace(message)})
}
