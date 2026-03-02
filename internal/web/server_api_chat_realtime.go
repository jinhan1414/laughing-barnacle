package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"laughing-barnacle/internal/conversation"
)

const chatStreamRetryMS = 3000

func (s *Server) handleAPIChatSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if s.turns == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "chat send unavailable"})
		return
	}
	var req apiChatSendRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid request body"})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid request body"})
		return
	}
	turn, err := s.turns.Submit(req.Message)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(apiChatSendResponse{
		OK:           true,
		MessageID:    turn.MessageID,
		TurnID:       turn.ID,
		AcceptedAtUS: turn.AcceptedAt.UnixMicro(),
	})
}

func (s *Server) handleAPIChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.convStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sinceUS, err := parseChatCursor(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	_, _ = fmt.Fprintf(w, "retry: %d\n\n", chatStreamRetryMS)
	flusher.Flush()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		updates := s.listChatUpdates(sinceUS)
		for _, item := range updates {
			if err := writeChatSSEEvent(w, item); err != nil {
				return
			}
			flusher.Flush()
			sinceUS = item.CreatedAtUS
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = io.WriteString(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) listChatUpdates(sinceUS int64) []apiChatUpdate {
	if s == nil || s.convStore == nil {
		return nil
	}
	_, messages, events := s.convStore.SnapshotWithEvents()
	type item struct {
		update apiChatUpdate
		seq    int
	}
	all := make([]item, 0, len(messages)+len(events))
	seq := 0
	for _, msg := range messages {
		if strings.TrimSpace(msg.Role) != "assistant" {
			continue
		}
		createdAtUS := msg.CreatedAt.UnixMicro()
		if createdAtUS <= sinceUS {
			continue
		}
		all = append(all, item{
			update: apiChatUpdate{
				Kind:        "assistant",
				Content:     msg.Content,
				CreatedAtUS: createdAtUS,
				Usage:       toAPITokenUsage(msg.Usage),
			},
			seq: seq,
		})
		seq++
	}
	for _, evt := range events {
		if !allowChatEventType(evt.Type) {
			continue
		}
		createdAtUS := evt.CreatedAt.UnixMicro()
		if createdAtUS <= sinceUS {
			continue
		}
		all = append(all, item{
			update: apiChatUpdate{
				Kind:        "event",
				EventType:   strings.TrimSpace(evt.Type),
				Content:     evt.Content,
				CreatedAtUS: createdAtUS,
			},
			seq: seq,
		})
		seq++
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].update.CreatedAtUS == all[j].update.CreatedAtUS {
			return all[i].seq < all[j].seq
		}
		return all[i].update.CreatedAtUS < all[j].update.CreatedAtUS
	})
	out := make([]apiChatUpdate, 0, len(all))
	for _, item := range all {
		out = append(out, item.update)
	}
	return out
}

func parseChatCursor(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("since_us"))
	if raw == "" {
		raw = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("chat cursor must be a non-negative integer")
	}
	return value, nil
}

func allowChatEventType(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "context_compression", "async_task_status", "autonomous_run_status", chatTurnEventType:
		return true
	default:
		return false
	}
}

func writeChatSSEEvent(w io.Writer, item apiChatUpdate) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		w,
		"id: %d\nevent: %s\ndata: %s\n\n",
		item.CreatedAtUS,
		chatSSEEventName(item),
		string(payload),
	)
	return err
}

func chatSSEEventName(item apiChatUpdate) string {
	if item.Kind == "assistant" {
		return "assistant.message"
	}
	switch item.EventType {
	case "async_task_status":
		return "task.status"
	case "autonomous_run_status":
		return "run.status"
	case chatTurnEventType:
		return "turn.status"
	case "context_compression":
		return "context.compression"
	default:
		return "chat.event"
	}
}

func toAPITokenUsage(usage *conversation.TokenUsage) *apiTokenUsage {
	if usage == nil {
		return nil
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 && usage.CachedTokens == 0 {
		return nil
	}
	return &apiTokenUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		CachedTokens:     usage.CachedTokens,
	}
}
