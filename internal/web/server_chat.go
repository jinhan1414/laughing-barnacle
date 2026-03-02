package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
)

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/chat", http.StatusFound)
}

func (s *Server) handleSettingsShortcut(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/settings?section=mcp", http.StatusFound)
}

func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	_, messages, events := s.convStore.SnapshotWithEvents()
	data := chatPageData{
		Timeline:       buildChatTimeline(messages, events),
		Error:          r.URL.Query().Get("error"),
		RetryAvailable: r.URL.Query().Get("retry") == "1",
		Draft:          r.URL.Query().Get("draft"),
	}
	_ = s.tmpl.ExecuteTemplate(w, "chat.html", data)
}

func (s *Server) handleChatGreet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.agent == nil || s.convStore == nil || s.mcpStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if s.hasRunningScheduledTask() {
		writeChatGreetJSON(w, chatGreetResponse{Created: false, Reason: "task_running"})
		return
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	lastDate := s.mcpStore.GetLastChatGreetingDate()
	lastAt := s.mcpStore.GetLastChatGreetingAt()
	isFirstToday := strings.TrimSpace(lastDate) != today

	if !isFirstToday {
		if lastAt.IsZero() {
			writeChatGreetJSON(w, chatGreetResponse{Created: false, Reason: "already_greeted_today"})
			return
		}
		if now.Before(lastAt) || now.Sub(lastAt) < chatGreetingCooldown {
			writeChatGreetJSON(w, chatGreetResponse{Created: false, Reason: "cooldown"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	greeting, err := s.agent.GenerateChatGreeting(ctx, agent.ChatGreetingInput{
		Now:                 now,
		IsFirstToday:        isFirstToday,
		LastGreetingAt:      lastAt,
		LastGreetingContent: s.mcpStore.GetLastChatGreetingContent(),
		RecentTaskStatuses:  buildRecentTaskStatusLines(s.mcpStore.ListScheduledTasks(), 3),
	})
	greeting = strings.TrimSpace(greeting)
	if err != nil || greeting == "" {
		greeting = fallbackChatGreeting(now)
	}

	s.convStore.Append("assistant", greeting)
	_ = s.mcpStore.SetLastChatGreetingState(today, now, greeting)

	writeChatGreetJSON(w, chatGreetResponse{
		Created: true,
		Content: greeting,
	})
}

func buildChatTimeline(messages []conversation.Message, events []conversation.Event) []chatTimelineItem {
	type timelineWithSeq struct {
		item chatTimelineItem
		seq  int
	}

	all := make([]timelineWithSeq, 0, len(messages)+len(events))
	asyncTaskEventIndex := make(map[string]int)
	turnEventIndex := make(map[string]int)
	seq := 0
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		kind := ""
		switch role {
		case "user":
			kind = "user"
		case "assistant":
			kind = "assistant"
		default:
			continue
		}
		all = append(all, timelineWithSeq{
			item: chatTimelineItem{
				Kind:      kind,
				Content:   msg.Content,
				ToolCalls: msg.ToolCalls,
				Usage:     msg.Usage,
				CreatedAt: msg.CreatedAt,
			},
			seq: seq,
		})
		seq++
	}

	for _, evt := range events {
		eventType := strings.TrimSpace(evt.Type)
		if !allowChatEventType(eventType) {
			continue
		}
		eventTaskID := ""
		eventTurnID := ""
		if eventType == "async_task_status" {
			eventTaskID = extractAsyncTaskStatusTaskID(evt.Content)
		}
		if eventType == chatTurnEventType {
			eventTurnID = extractChatTurnStatusTurnID(evt.Content)
		}
		next := timelineWithSeq{
			item: chatTimelineItem{
				Kind:        "event",
				EventType:   eventType,
				EventTaskID: eventTaskID,
				EventTurnID: eventTurnID,
				Content:     evt.Content,
				CreatedAt:   evt.CreatedAt,
			},
			seq: seq,
		}
		seq++
		if eventTaskID != "" {
			if idx, ok := asyncTaskEventIndex[eventTaskID]; ok {
				all[idx] = next
				continue
			}
		}
		if eventTurnID != "" {
			if idx, ok := turnEventIndex[eventTurnID]; ok {
				all[idx] = next
				continue
			}
		}
		all = append(all, next)
		if eventTaskID != "" {
			asyncTaskEventIndex[eventTaskID] = len(all) - 1
		}
		if eventTurnID != "" {
			turnEventIndex[eventTurnID] = len(all) - 1
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		left := all[i].item.CreatedAt
		right := all[j].item.CreatedAt
		if left.Equal(right) {
			return all[i].seq < all[j].seq
		}
		return left.Before(right)
	})

	out := make([]chatTimelineItem, 0, len(all))
	now := time.Now()
	var previousAt time.Time
	for _, it := range all {
		item := it.item
		if shouldShowChatTimestamp(previousAt, item.CreatedAt) {
			item.ShowTimestamp = true
			item.TimestampLabel = formatChatTimestamp(item.CreatedAt, now)
		}
		out = append(out, item)
		if !item.CreatedAt.IsZero() {
			previousAt = item.CreatedAt
		}
	}
	return out
}

func shouldShowChatTimestamp(previous, current time.Time) bool {
	if current.IsZero() {
		return false
	}
	if previous.IsZero() {
		return true
	}
	previousLocal := previous.Local()
	currentLocal := current.Local()
	if !sameLocalDay(previousLocal, currentLocal) {
		return true
	}
	return currentLocal.Sub(previousLocal) >= chatTimestampGap
}

func formatChatTimestamp(current, now time.Time) string {
	if current.IsZero() {
		return ""
	}
	currentLocal := current.Local()
	if now.IsZero() {
		now = time.Now()
	}
	nowLocal := now.Local()
	if sameLocalDay(currentLocal, nowLocal) {
		return currentLocal.Format("15:04")
	}
	if sameLocalDay(currentLocal, nowLocal.AddDate(0, 0, -1)) {
		return "昨天 " + currentLocal.Format("15:04")
	}
	if currentLocal.Year() == nowLocal.Year() {
		return fmt.Sprintf("%d月%d日 %s", int(currentLocal.Month()), currentLocal.Day(), currentLocal.Format("15:04"))
	}
	return fmt.Sprintf("%d年%d月%d日 %s", currentLocal.Year(), int(currentLocal.Month()), currentLocal.Day(), currentLocal.Format("15:04"))
}

func sameLocalDay(left, right time.Time) bool {
	return left.Year() == right.Year() &&
		left.Month() == right.Month() &&
		left.Day() == right.Day()
}

func extractAsyncTaskStatusTaskID(content string) string {
	return extractStatusValue(content, "task_id=")
}

func extractChatTurnStatusTurnID(content string) string {
	return extractStatusValue(content, "turn_id=")
}

func extractStatusValue(content, prefix string) string {
	for _, part := range strings.Split(content, "|") {
		text := strings.TrimSpace(part)
		if !strings.HasPrefix(text, prefix) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(text, prefix))
	}
	return ""
}

func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/chat?error="+url.QueryEscape("请求参数解析失败"), http.StatusFound)
		return
	}

	message := strings.TrimSpace(r.FormValue("message"))
	if message == "" {
		http.Redirect(w, r, "/chat", http.StatusFound)
		return
	}

	if _, err := s.agent.HandleUserMessage(r.Context(), message); err != nil {
		query := url.Values{}
		query.Set("error", err.Error())
		query.Set("retry", "1")
		query.Set("draft", message)
		http.Redirect(w, r, "/chat?"+query.Encode(), http.StatusFound)
		return
	}

	http.Redirect(w, r, "/chat", http.StatusFound)
}

func (s *Server) handleChatRetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if _, err := s.agent.RetryLastUserMessage(r.Context()); err != nil {
		query := url.Values{}
		query.Set("error", err.Error())
		query.Set("retry", "1")
		http.Redirect(w, r, "/chat?"+query.Encode(), http.StatusFound)
		return
	}

	http.Redirect(w, r, "/chat", http.StatusFound)
}
