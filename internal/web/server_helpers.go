package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"laughing-barnacle/internal/memory"
	"laughing-barnacle/internal/routine"
	"laughing-barnacle/internal/scheduler"
)

func (s *Server) redirectSettings(w http.ResponseWriter, r *http.Request, section, success, failure string) {
	values := url.Values{}
	if strings.TrimSpace(section) == "" {
		section = "mcp"
	}
	values.Set("section", section)
	if strings.TrimSpace(success) != "" {
		values.Set("success", success)
	}
	if strings.TrimSpace(failure) != "" {
		values.Set("error", failure)
	}
	http.Redirect(w, r, "/settings?"+values.Encode(), http.StatusFound)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func writeChatGreetJSON(w http.ResponseWriter, payload chatGreetResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) hasRunningScheduledTask() bool {
	if s.scheduler == nil {
		return false
	}
	checker, ok := s.scheduler.(scheduleRunningChecker)
	if !ok {
		return false
	}
	return checker.HasRunningTask()
}

func buildRecentTaskStatusLines(tasks []scheduler.Task, limit int) []string {
	if limit <= 0 || len(tasks) == 0 {
		return nil
	}
	type taskRun struct {
		id      string
		status  string
		message string
		runAt   time.Time
	}
	runs := make([]taskRun, 0, len(tasks))
	for _, task := range tasks {
		if task.LastRunAt.IsZero() {
			continue
		}
		runs = append(runs, taskRun{
			id:      strings.TrimSpace(task.ID),
			status:  strings.TrimSpace(task.LastStatus),
			message: strings.TrimSpace(task.LastMessage),
			runAt:   task.LastRunAt,
		})
	}
	if len(runs) == 0 {
		return nil
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if runs[i].runAt.Equal(runs[j].runAt) {
			return runs[i].id < runs[j].id
		}
		return runs[i].runAt.After(runs[j].runAt)
	})
	if len(runs) > limit {
		runs = runs[:limit]
	}

	out := make([]string, 0, len(runs))
	for _, run := range runs {
		line := strings.TrimSpace(run.id)
		if line == "" {
			line = "(unknown_task)"
		}
		if run.status != "" {
			line += " | " + run.status
		}
		line += " | " + run.runAt.Format("2006-01-02 15:04:05")
		if run.message != "" {
			line += " | " + run.message
		}
		out = append(out, line)
	}
	return out
}

func findMemoryRefValue(refs []memory.Ref, kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return ""
	}
	for _, ref := range refs {
		if strings.ToLower(strings.TrimSpace(ref.Kind)) == kind {
			return strings.TrimSpace(ref.Value)
		}
	}
	return ""
}

func latestMaintenanceAudit(entries []memory.AuditEntry) (memory.AuditEntry, bool) {
	var best memory.AuditEntry
	found := false
	for _, entry := range entries {
		if !strings.EqualFold(strings.TrimSpace(entry.Action), "maintenance") {
			continue
		}
		if !found || entry.CreatedAt.After(best.CreatedAt) || (entry.CreatedAt.Equal(best.CreatedAt) && entry.ID > best.ID) {
			best = entry
			found = true
		}
	}
	return best, found
}

func fallbackChatGreeting(now time.Time) string {
	hour := now.Hour()
	switch {
	case hour < 6:
		return "夜里好，欢迎回来。我在这，随时可以继续。"
	case hour < 12:
		return "早上好，欢迎回来。我在这，随时可以继续。"
	case hour < 18:
		return "下午好，欢迎回来。我在这，随时可以继续。"
	default:
		return "晚上好，欢迎回来。我在这，随时可以继续。"
	}
}

func displayTransport(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sse":
		return "sse"
	case "stdio":
		return "stdio"
	default:
		return "streamable_http"
	}
}

func findScheduledTaskByID(tasks []scheduler.Task, id string) (scheduler.Task, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return scheduler.Task{}, false
	}
	for _, task := range tasks {
		if strings.TrimSpace(task.ID) == id {
			return task, true
		}
	}
	return scheduler.Task{}, false
}

func (s *Server) validateScheduleActionSkill(action string) error {
	if s.skillStore == nil {
		return nil
	}
	skillID, ok := routine.SkillIDFromAction(action)
	if !ok {
		return nil
	}
	if _, found := s.skillStore.ReadEnabledSkillPrompt(skillID); found {
		return nil
	}
	return fmt.Errorf("action 引用的 skill %q 不存在或未启用", skillID)
}

func displayScheduleAction(action string) string {
	action = routine.NormalizeAction(strings.TrimSpace(action))
	switch action {
	case routine.ActionNightReflectionEvolution:
		return "夜间复盘与进化"
	case routine.ActionMorningPlanning:
		return "晨间规划"
	default:
		if skillID, ok := routine.SkillIDFromAction(action); ok {
			return "Skill 执行：" + skillID
		}
		return action
	}
}

func parseJSONArgsList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("stdio 参数必须是 JSON 字符串数组，例如 [\"-y\",\"@modelcontextprotocol/server-filesystem\"]")
	}

	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
