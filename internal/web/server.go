package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llmlog"
	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/routine"
	"laughing-barnacle/internal/scheduler"
	"laughing-barnacle/internal/skills"
)

//go:embed templates/*.html
var embeddedTemplates embed.FS

type Server struct {
	agent      *agent.Agent
	convStore  *conversation.Store
	logStore   *llmlog.Store
	mcpStore   *mcp.Store
	mcpTools   *mcp.ToolProvider
	skillStore *skills.Store
	scheduler  ScheduleReloader
	tmpl       *template.Template
}

type ScheduleReloader interface {
	Reload() error
	RunNow(taskID string) error
}

type scheduleRunningChecker interface {
	HasRunningTask() bool
}

type chatPageData struct {
	Timeline       []chatTimelineItem
	Error          string
	RetryAvailable bool
	Draft          string
}

type chatTimelineItem struct {
	Kind      string
	Content   string
	ToolCalls []conversation.ToolCall
	CreatedAt time.Time
}

type logsPageData struct {
	Entries []llmlog.Entry
}

type settingsSection struct {
	Key         string
	Title       string
	Description string
}

type mcpServiceView struct {
	ID          string
	Name        string
	Endpoint    string
	Command     string
	Args        string
	Transport   string
	Enabled     bool
	UpdatedAt   string
	Connected   bool
	ToolCount   int
	Tools       []mcpServiceToolView
	StatusLabel string
	StatusError string
}

type mcpServiceToolView struct {
	Name        string
	Description string
	Enabled     bool
}

type settingsPageData struct {
	ActiveSection string
	Sections      []settingsSection
	Services      []mcpServiceView
	Skills        []skillView
	Schedules     []scheduledTaskView
	AgentPrompts  agentPromptsView
	Success       string
	Error         string
}

type skillView struct {
	ID          string
	Name        string
	Description string
	Prompt      string
	Source      string
	Enabled     bool
	UpdatedAt   string
}

type agentPromptsView struct {
	SystemPrompt            string
	CompressionSystemPrompt string
	UpdatedAt               string
}

type scheduledTaskView struct {
	ID          string
	Name        string
	Description string
	Action      string
	ActionLabel string
	CronExpr    string
	Enabled     bool
	UpdatedAt   string
	LastRunAt   string
	LastStatus  string
	LastMessage string
}

type apiMCPService struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Transport string    `json:"transport"`
	Endpoint  string    `json:"endpoint,omitempty"`
	Command   string    `json:"command,omitempty"`
	Args      []string  `json:"args,omitempty"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type apiSkill struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Source      string    `json:"source,omitempty"`
	Enabled     bool      `json:"enabled"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type apiScheduledTask struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Action      string    `json:"action"`
	CronExpr    string    `json:"cron_expr"`
	Enabled     bool      `json:"enabled"`
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
	LastStatus  string    `json:"last_status,omitempty"`
	LastMessage string    `json:"last_message,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type chatGreetResponse struct {
	Created bool   `json:"created"`
	Content string `json:"content,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

const chatGreetingCooldown = 30 * time.Minute

func NewServer(
	agent *agent.Agent,
	convStore *conversation.Store,
	logStore *llmlog.Store,
	mcpStore *mcp.Store,
	mcpTools *mcp.ToolProvider,
	skillStore *skills.Store,
	scheduler ScheduleReloader,
) (*Server, error) {
	tmpl, err := template.ParseFS(embeddedTemplates, "templates/*.html")
	if err != nil {
		return nil, err
	}

	return &Server{
		agent:      agent,
		convStore:  convStore,
		logStore:   logStore,
		mcpStore:   mcpStore,
		mcpTools:   mcpTools,
		skillStore: skillStore,
		scheduler:  scheduler,
		tmpl:       tmpl,
	}, nil
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/chat", s.handleChatPage)
	mux.HandleFunc("/chat/greet", s.handleChatGreet)
	mux.HandleFunc("/chat/send", s.handleChatSend)
	mux.HandleFunc("/chat/retry", s.handleChatRetry)
	mux.HandleFunc("/chat/reset", s.handleChatReset)
	mux.HandleFunc("/chat/settings", s.handleSettingsShortcut)
	mux.HandleFunc("/config", s.handleSettingsShortcut)
	mux.HandleFunc("/logs", s.handleLogsPage)
	mux.HandleFunc("/settings", s.handleSettingsPage)
	mux.HandleFunc("/settings/mcp/save", s.handleSettingsMCPSave)
	mux.HandleFunc("/settings/mcp/delete", s.handleSettingsMCPDelete)
	mux.HandleFunc("/settings/mcp/toggle", s.handleSettingsMCPToggle)
	mux.HandleFunc("/settings/mcp/tool/toggle", s.handleSettingsMCPToolToggle)
	mux.HandleFunc("/settings/skills/install", s.handleSettingsSkillInstall)
	mux.HandleFunc("/settings/skills/save", s.handleSettingsSkillSave)
	mux.HandleFunc("/settings/skills/delete", s.handleSettingsSkillDelete)
	mux.HandleFunc("/settings/skills/toggle", s.handleSettingsSkillToggle)
	mux.HandleFunc("/settings/schedules/save", s.handleSettingsScheduleSave)
	mux.HandleFunc("/settings/schedules/toggle", s.handleSettingsScheduleToggle)
	mux.HandleFunc("/settings/schedules/run", s.handleSettingsScheduleRun)
	mux.HandleFunc("/settings/llm/prompts/save", s.handleSettingsLLMPromptsSave)
	mux.HandleFunc("/settings/llm/prompts/reset", s.handleSettingsLLMPromptsReset)
	mux.HandleFunc("/api/mcp/services", s.handleAPIMCPServices)
	mux.HandleFunc("/api/skills", s.handleAPISkills)
	mux.HandleFunc("/api/skills/read", s.handleAPISkillRead)
	mux.HandleFunc("/api/skills/catalog/search", s.handleAPISkillsCatalogSearch)
	mux.HandleFunc("/api/schedules", s.handleAPISchedules)
	mux.HandleFunc("/api/context/archive/index", s.handleAPIContextArchiveIndex)
	mux.HandleFunc("/api/context/archive/section", s.handleAPIContextArchiveSection)
	mux.HandleFunc("/healthz", s.handleHealthz)
}

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
	for _, it := range all {
		out = append(out, it.item)
	}
	return out
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

func (s *Server) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	data := logsPageData{Entries: s.logStore.List()}
	_ = s.tmpl.ExecuteTemplate(w, "logs.html", data)
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	section := strings.TrimSpace(r.URL.Query().Get("section"))
	if section == "" {
		section = "mcp"
	}
	if section != "mcp" && section != "llm" && section != "security" && section != "skills" && section != "schedules" {
		section = "mcp"
	}

	data := settingsPageData{
		ActiveSection: section,
		Sections: []settingsSection{
			{Key: "mcp", Title: "MCP 服务", Description: "管理 Agent 可用的 MCP 工具服务"},
			{Key: "schedules", Title: "定时任务", Description: "统一管理系统 Cron 定时任务"},
			{Key: "llm", Title: "提示词策略", Description: "配置 Agent 系统提示词与压缩提示词"},
			{Key: "security", Title: "安全策略", Description: "预留：权限与审计配置"},
			{Key: "skills", Title: "Skill 技能", Description: "配置 Agent 的可复用技能指令"},
		},
		Success: r.URL.Query().Get("success"),
		Error:   r.URL.Query().Get("error"),
	}

	if section == "mcp" {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		statuses := s.mcpTools.ListServiceStatuses(ctx)
		data.Services = make([]mcpServiceView, 0, len(statuses))
		for _, status := range statuses {
			view := mcpServiceView{
				ID:        status.Service.ID,
				Name:      status.Service.Name,
				Endpoint:  status.Service.Endpoint,
				Command:   status.Service.Command,
				Args:      strings.Join(status.Service.Args, " "),
				Transport: displayTransport(status.Service.Transport),
				Enabled:   status.Service.Enabled,
				UpdatedAt: status.Service.UpdatedAt.Format("2006-01-02 15:04:05"),
			}
			switch {
			case !status.Service.Enabled:
				view.StatusLabel = "已禁用"
			case status.Connected:
				view.Connected = true
				view.StatusLabel = "连接正常"
				view.ToolCount = status.ToolCount
				view.Tools = make([]mcpServiceToolView, 0, len(status.Tools))
				for _, tool := range status.Tools {
					view.Tools = append(view.Tools, mcpServiceToolView{
						Name:        tool.Name,
						Description: tool.Description,
						Enabled:     tool.Enabled,
					})
				}
			default:
				view.StatusLabel = "连接失败"
				view.StatusError = status.Error
			}
			data.Services = append(data.Services, view)
		}
	} else if section == "skills" {
		allSkills := s.skillStore.ListSkills()
		data.Skills = make([]skillView, 0, len(allSkills))
		for _, skill := range allSkills {
			view := skillView{
				ID:          skill.ID,
				Name:        skill.Name,
				Description: skill.Description,
				Prompt:      skill.Prompt,
				Source:      skill.Source,
				Enabled:     skill.Enabled,
			}
			if !skill.UpdatedAt.IsZero() {
				view.UpdatedAt = skill.UpdatedAt.Format("2006-01-02 15:04:05")
			}
			data.Skills = append(data.Skills, view)
		}
	} else if section == "llm" {
		cfg := s.mcpStore.GetAgentPromptConfig()
		data.AgentPrompts = agentPromptsView{
			SystemPrompt:            cfg.SystemPrompt,
			CompressionSystemPrompt: cfg.CompressionSystemPrompt,
		}
		if !cfg.UpdatedAt.IsZero() {
			data.AgentPrompts.UpdatedAt = cfg.UpdatedAt.Format("2006-01-02 15:04:05")
		}
	} else if section == "schedules" {
		allTasks := s.mcpStore.ListScheduledTasks()
		data.Schedules = make([]scheduledTaskView, 0, len(allTasks))
		for _, task := range allTasks {
			view := scheduledTaskView{
				ID:          task.ID,
				Name:        task.Name,
				Description: task.Description,
				Action:      task.Action,
				ActionLabel: displayScheduleAction(task.Action),
				CronExpr:    task.CronExpr,
				Enabled:     task.Enabled,
				LastStatus:  strings.TrimSpace(task.LastStatus),
				LastMessage: strings.TrimSpace(task.LastMessage),
			}
			if !task.UpdatedAt.IsZero() {
				view.UpdatedAt = task.UpdatedAt.Format("2006-01-02 15:04:05")
			}
			if !task.LastRunAt.IsZero() {
				view.LastRunAt = task.LastRunAt.Format("2006-01-02 15:04:05")
			}
			data.Schedules = append(data.Schedules, view)
		}
	}

	_ = s.tmpl.ExecuteTemplate(w, "settings.html", data)
}

func (s *Server) handleSettingsMCPSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "mcp", "", "请求参数解析失败")
		return
	}

	service := mcp.Service{
		ID:        "",
		Name:      strings.TrimSpace(r.FormValue("name")),
		Endpoint:  strings.TrimSpace(r.FormValue("endpoint")),
		Command:   strings.TrimSpace(r.FormValue("command")),
		Transport: strings.TrimSpace(r.FormValue("transport")),
		AuthToken: strings.TrimSpace(r.FormValue("auth_token")),
		Enabled:   r.FormValue("enabled") == "on",
	}
	args, err := parseJSONArgsList(strings.TrimSpace(r.FormValue("args_json")))
	if err != nil {
		s.redirectSettings(w, r, "mcp", "", err.Error())
		return
	}
	service.Args = args
	if err := s.mcpStore.UpsertService(service); err != nil {
		s.redirectSettings(w, r, "mcp", "", err.Error())
		return
	}
	s.mcpTools.InvalidateCache()
	s.redirectSettings(w, r, "mcp", "MCP 服务已保存", "")
}

func (s *Server) handleSettingsMCPDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "mcp", "", "请求参数解析失败")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if err := s.mcpStore.DeleteService(id); err != nil {
		s.redirectSettings(w, r, "mcp", "", err.Error())
		return
	}
	s.mcpTools.InvalidateCache()
	s.redirectSettings(w, r, "mcp", fmt.Sprintf("MCP 服务 %s 已删除", id), "")
}

func (s *Server) handleSettingsMCPToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "mcp", "", "请求参数解析失败")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	enable := r.FormValue("enabled") == "true"
	if err := s.mcpStore.SetEnabled(id, enable); err != nil {
		s.redirectSettings(w, r, "mcp", "", err.Error())
		return
	}
	s.mcpTools.InvalidateCache()
	if enable {
		s.redirectSettings(w, r, "mcp", fmt.Sprintf("MCP 服务 %s 已启用", id), "")
		return
	}
	s.redirectSettings(w, r, "mcp", fmt.Sprintf("MCP 服务 %s 已禁用", id), "")
}

func (s *Server) handleSettingsMCPToolToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "mcp", "", "请求参数解析失败")
		return
	}
	serviceID := strings.TrimSpace(r.FormValue("service_id"))
	toolName := strings.TrimSpace(r.FormValue("tool_name"))
	enable := r.FormValue("enabled") == "true"
	if err := s.mcpStore.SetServiceToolEnabled(serviceID, toolName, enable); err != nil {
		s.redirectSettings(w, r, "mcp", "", err.Error())
		return
	}
	s.mcpTools.InvalidateCache()
	if enable {
		s.redirectSettings(w, r, "mcp", fmt.Sprintf("工具 %s 已启用", toolName), "")
		return
	}
	s.redirectSettings(w, r, "mcp", fmt.Sprintf("工具 %s 已禁用", toolName), "")
}

func (s *Server) handleSettingsSkillInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "skills", "", "请求参数解析失败")
		return
	}

	rawURL := strings.TrimSpace(r.FormValue("skills_sh_url"))
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	installed, err := s.skillStore.InstallFromSkillsSH(ctx, rawURL)
	if err != nil {
		s.redirectSettings(w, r, "skills", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "skills", fmt.Sprintf("Skill 已安装：%s (%s)", installed.Name, installed.ID), "")
}

func (s *Server) handleSettingsSkillSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "skills", "", "请求参数解析失败")
		return
	}

	skill := skills.Skill{
		ID:          "",
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Prompt:      strings.TrimSpace(r.FormValue("prompt")),
		Enabled:     r.FormValue("enabled") == "on",
	}
	if err := s.skillStore.UpsertSkill(skill); err != nil {
		s.redirectSettings(w, r, "skills", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "skills", "Skill 已保存", "")
}

func (s *Server) handleSettingsSkillDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "skills", "", "请求参数解析失败")
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	if err := s.skillStore.DeleteSkill(id); err != nil {
		s.redirectSettings(w, r, "skills", "", err.Error())
		return
	}
	s.redirectSettings(w, r, "skills", fmt.Sprintf("Skill %s 已删除", id), "")
}

func (s *Server) handleSettingsSkillToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "skills", "", "请求参数解析失败")
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	enable := r.FormValue("enabled") == "true"
	if err := s.skillStore.SetSkillEnabled(id, enable); err != nil {
		s.redirectSettings(w, r, "skills", "", err.Error())
		return
	}
	if enable {
		s.redirectSettings(w, r, "skills", fmt.Sprintf("Skill %s 已启用", id), "")
		return
	}
	s.redirectSettings(w, r, "skills", fmt.Sprintf("Skill %s 已禁用", id), "")
}

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
		Enabled:     r.FormValue("enabled") == "on",
	}
	if err := s.validateScheduleActionSkill(task.Action); err != nil {
		s.redirectSettings(w, r, "schedules", "", err.Error())
		return
	}
	if err := s.mcpStore.UpsertScheduledTask(task); err != nil {
		s.redirectSettings(w, r, "schedules", "", err.Error())
		return
	}
	if s.scheduler != nil {
		if err := s.scheduler.Reload(); err != nil {
			s.redirectSettings(w, r, "schedules", "", "调度器重载失败："+err.Error())
			return
		}
	}
	s.redirectSettings(w, r, "schedules", "定时任务已保存", "")
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
	enable := r.FormValue("enabled") == "true"
	if err := s.mcpStore.SetScheduledTaskEnabled(id, enable); err != nil {
		s.redirectSettings(w, r, "schedules", "", err.Error())
		return
	}
	if s.scheduler != nil {
		if err := s.scheduler.Reload(); err != nil {
			s.redirectSettings(w, r, "schedules", "", "调度器重载失败："+err.Error())
			return
		}
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
	if taskID == "" {
		s.redirectSettings(w, r, "schedules", "", "任务 id 不能为空")
		return
	}
	task, ok := findScheduledTaskByID(s.mcpStore.ListScheduledTasks(), taskID)
	if !ok {
		s.redirectSettings(w, r, "schedules", "", fmt.Sprintf("定时任务 %s 不存在", taskID))
		return
	}
	if err := s.validateScheduleActionSkill(task.Action); err != nil {
		runAt := time.Now().Truncate(time.Second)
		_ = s.mcpStore.MarkScheduledTaskRun(task.ID, runAt, "error", err.Error())
		s.redirectSettings(w, r, "schedules", "", "立即执行失败："+err.Error())
		return
	}
	if s.scheduler != nil {
		if err := s.scheduler.RunNow(taskID); err != nil {
			s.redirectSettings(w, r, "schedules", "", "立即执行失败："+err.Error())
			return
		}
		s.redirectSettings(w, r, "schedules", fmt.Sprintf("定时任务 %s 已立即执行", taskID), "")
		return
	}
	if s.agent == nil {
		s.redirectSettings(w, r, "schedules", "", "agent 未初始化")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	runAt := time.Now().Truncate(time.Second)
	if err := s.agent.RunScheduledTask(ctx, task.Action); err != nil {
		_ = s.mcpStore.MarkScheduledTaskRun(task.ID, runAt, "error", err.Error())
		s.redirectSettings(w, r, "schedules", "", "立即执行失败："+err.Error())
		return
	}
	if err := s.mcpStore.MarkScheduledTaskRun(task.ID, runAt, "success", "manual_run"); err != nil {
		s.redirectSettings(w, r, "schedules", "", "状态写回失败："+err.Error())
		return
	}
	s.redirectSettings(w, r, "schedules", fmt.Sprintf("定时任务 %s 已立即执行", taskID), "")
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

func (s *Server) handleAPIMCPServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	services := s.mcpStore.ListServices()
	items := make([]apiMCPService, 0, len(services))
	for _, svc := range services {
		items = append(items, apiMCPService{
			ID:        svc.ID,
			Name:      svc.Name,
			Transport: strings.TrimSpace(svc.Transport),
			Endpoint:  strings.TrimSpace(svc.Endpoint),
			Command:   strings.TrimSpace(svc.Command),
			Args:      append([]string(nil), svc.Args...),
			Enabled:   svc.Enabled,
			UpdatedAt: svc.UpdatedAt,
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

func (s *Server) handleAPISkillRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	skillID := strings.TrimSpace(r.URL.Query().Get("id"))
	if skillID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "query parameter id is required"})
		return
	}

	prompt, ok := s.skillStore.ReadEnabledSkillPrompt(skillID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "skill not found or not enabled"})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":     skillID,
		"prompt": strings.TrimSpace(prompt),
	})
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

func (s *Server) handleAPIContextArchiveIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	archiveID := strings.TrimSpace(r.URL.Query().Get("archive_id"))
	if archiveID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "query parameter archive_id is required"})
		return
	}
	index, err := s.convStore.ReadArchiveIndex(archiveID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, conversation.ErrArchiveNotFound) {
			status = http.StatusNotFound
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(index)
}

func (s *Server) handleAPIContextArchiveSection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	archiveID := strings.TrimSpace(r.URL.Query().Get("archive_id"))
	sectionID := strings.TrimSpace(r.URL.Query().Get("section_id"))
	if archiveID == "" || sectionID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "query parameter archive_id and section_id are required"})
		return
	}
	section, err := s.convStore.ReadArchiveSection(archiveID, sectionID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, conversation.ErrArchiveNotFound) || errors.Is(err, conversation.ErrArchiveSectionNotFound) {
			status = http.StatusNotFound
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(section)
}

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
		return "streamableHttp"
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
