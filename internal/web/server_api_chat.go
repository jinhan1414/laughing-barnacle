package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"laughing-barnacle/internal/conversation"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleAPIChatUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.convStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	sinceUS := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("since_us")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": "query parameter since_us must be a non-negative integer",
			})
			return
		}
		sinceUS = parsed
	}

	_, messages, events := s.convStore.SnapshotWithEvents()
	timeline := buildChatTimeline(messages, events)
	updates := make([]apiChatUpdate, 0, len(timeline))
	for _, item := range timeline {
		if item.Kind != "assistant" && item.Kind != "event" {
			continue
		}
		createdAtUS := item.CreatedAt.UnixMicro()
		if createdAtUS <= sinceUS {
			continue
		}
		updates = append(updates, apiChatUpdate{
			Kind:        item.Kind,
			EventType:   item.EventType,
			Content:     item.Content,
			CreatedAtUS: createdAtUS,
			Usage:       toAPITokenUsage(item.Usage),
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"updates": updates,
	})
}

func toAPITokenUsage(usage *conversation.TokenUsage) *apiTokenUsage {
	if usage == nil {
		return nil
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return &apiTokenUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

func (s *Server) handleAPIChatToolRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if s.agent == nil || s.convStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "chat tool unavailable",
		})
		return
	}

	var req apiChatToolRunRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid request body",
		})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid request body",
		})
		return
	}

	tool := strings.TrimSpace(req.Tool)
	if tool == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "tool is required",
		})
		return
	}

	switch tool {
	case "context_compress":
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		_, compressed, err := s.agent.CompressContextNow(ctx)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": "run context compress failed: " + err.Error(),
			})
			return
		}
		message := "当前没有可压缩的上下文"
		if compressed {
			message = "已触发上下文压缩"
		}
		_ = json.NewEncoder(w).Encode(apiChatToolRunResponse{
			OK:         true,
			Tool:       tool,
			Compressed: compressed,
			Message:    message,
		})
	default:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": fmt.Sprintf("unknown chat tool: %s", tool),
		})
	}
}
