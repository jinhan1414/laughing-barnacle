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
		if strings.TrimSpace(evt.Type) != "context_compression" {
			continue
		}
		all = append(all, timelineWithSeq{
			item: chatTimelineItem{
				Kind:      "event",
				Content:   evt.Content,
				CreatedAt: evt.CreatedAt,
			},
			seq: seq,
		})
		seq++
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

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	if _, err := s.agent.HandleUserMessage(ctx, message); err != nil {
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

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	if _, err := s.agent.RetryLastUserMessage(ctx); err != nil {
		query := url.Values{}
		query.Set("error", err.Error())
		query.Set("retry", "1")
		http.Redirect(w, r, "/chat?"+query.Encode(), http.StatusFound)
		return
	}

	http.Redirect(w, r, "/chat", http.StatusFound)
}

func (s *Server) handleChatReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := s.convStore.Reset(); err != nil {
		http.Redirect(w, r, "/chat?error="+url.QueryEscape("重置上下文失败："+err.Error()), http.StatusFound)
		return
	}
	if err := s.logStore.Clear(); err != nil {
		http.Redirect(w, r, "/chat?error="+url.QueryEscape("清空日志失败："+err.Error()), http.StatusFound)
		return
	}

	http.Redirect(w, r, "/chat", http.StatusFound)
}
