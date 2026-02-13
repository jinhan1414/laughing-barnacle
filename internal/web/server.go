package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llmlog"
	"laughing-barnacle/internal/mcp"
)

//go:embed templates/*.html
var embeddedTemplates embed.FS

type Server struct {
	agent     *agent.Agent
	convStore *conversation.Store
	logStore  *llmlog.Store
	mcpStore  *mcp.Store
	mcpTools  *mcp.ToolProvider
	tmpl      *template.Template
}

type chatPageData struct {
	Summary        string
	Messages       []conversation.Message
	Error          string
	RetryAvailable bool
	Draft          string
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
	Transport   string
	Enabled     bool
	UpdatedAt   string
	Connected   bool
	ToolCount   int
	StatusLabel string
	StatusError string
}

type settingsPageData struct {
	ActiveSection string
	Sections      []settingsSection
	Services      []mcpServiceView
	Skills        []skillView
	Success       string
	Error         string
}

type skillView struct {
	ID        string
	Name      string
	Prompt    string
	Enabled   bool
	UpdatedAt string
}

func NewServer(
	agent *agent.Agent,
	convStore *conversation.Store,
	logStore *llmlog.Store,
	mcpStore *mcp.Store,
	mcpTools *mcp.ToolProvider,
) (*Server, error) {
	tmpl, err := template.ParseFS(embeddedTemplates, "templates/*.html")
	if err != nil {
		return nil, err
	}

	return &Server{
		agent:     agent,
		convStore: convStore,
		logStore:  logStore,
		mcpStore:  mcpStore,
		mcpTools:  mcpTools,
		tmpl:      tmpl,
	}, nil
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/chat", s.handleChatPage)
	mux.HandleFunc("/chat/send", s.handleChatSend)
	mux.HandleFunc("/chat/retry", s.handleChatRetry)
	mux.HandleFunc("/chat/settings", s.handleSettingsShortcut)
	mux.HandleFunc("/config", s.handleSettingsShortcut)
	mux.HandleFunc("/logs", s.handleLogsPage)
	mux.HandleFunc("/settings", s.handleSettingsPage)
	mux.HandleFunc("/settings/mcp/save", s.handleSettingsMCPSave)
	mux.HandleFunc("/settings/mcp/delete", s.handleSettingsMCPDelete)
	mux.HandleFunc("/settings/mcp/toggle", s.handleSettingsMCPToggle)
	mux.HandleFunc("/settings/skills/save", s.handleSettingsSkillSave)
	mux.HandleFunc("/settings/skills/delete", s.handleSettingsSkillDelete)
	mux.HandleFunc("/settings/skills/toggle", s.handleSettingsSkillToggle)
	mux.HandleFunc("/healthz", s.handleHealthz)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/chat", http.StatusFound)
}

func (s *Server) handleSettingsShortcut(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/settings?section=mcp", http.StatusFound)
}

func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	summary, messages := s.convStore.Snapshot()
	data := chatPageData{
		Summary:        summary,
		Messages:       messages,
		Error:          r.URL.Query().Get("error"),
		RetryAvailable: r.URL.Query().Get("retry") == "1",
		Draft:          r.URL.Query().Get("draft"),
	}
	_ = s.tmpl.ExecuteTemplate(w, "chat.html", data)
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

func (s *Server) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	data := logsPageData{Entries: s.logStore.List()}
	_ = s.tmpl.ExecuteTemplate(w, "logs.html", data)
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	section := strings.TrimSpace(r.URL.Query().Get("section"))
	if section == "" {
		section = "mcp"
	}
	if section != "mcp" && section != "llm" && section != "security" && section != "skills" {
		section = "mcp"
	}

	data := settingsPageData{
		ActiveSection: section,
		Sections: []settingsSection{
			{Key: "mcp", Title: "MCP 服务", Description: "管理 Agent 可用的 MCP 工具服务"},
			{Key: "llm", Title: "模型策略", Description: "预留：模型与路由策略配置"},
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
			default:
				view.StatusLabel = "连接失败"
				view.StatusError = status.Error
			}
			data.Services = append(data.Services, view)
		}
	} else if section == "skills" {
		skills := s.mcpStore.ListSkills()
		data.Skills = make([]skillView, 0, len(skills))
		for _, skill := range skills {
			view := skillView{
				ID:      skill.ID,
				Name:    skill.Name,
				Prompt:  skill.Prompt,
				Enabled: skill.Enabled,
			}
			if !skill.UpdatedAt.IsZero() {
				view.UpdatedAt = skill.UpdatedAt.Format("2006-01-02 15:04:05")
			}
			data.Skills = append(data.Skills, view)
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
		Transport: strings.TrimSpace(r.FormValue("transport")),
		AuthToken: strings.TrimSpace(r.FormValue("auth_token")),
		Enabled:   r.FormValue("enabled") == "on",
	}
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

func (s *Server) handleSettingsSkillSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectSettings(w, r, "skills", "", "请求参数解析失败")
		return
	}

	skill := mcp.Skill{
		ID:      "",
		Name:    strings.TrimSpace(r.FormValue("name")),
		Prompt:  strings.TrimSpace(r.FormValue("prompt")),
		Enabled: r.FormValue("enabled") == "on",
	}
	if err := s.mcpStore.UpsertSkill(skill); err != nil {
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
	if err := s.mcpStore.DeleteSkill(id); err != nil {
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
	if err := s.mcpStore.SetSkillEnabled(id, enable); err != nil {
		s.redirectSettings(w, r, "skills", "", err.Error())
		return
	}
	if enable {
		s.redirectSettings(w, r, "skills", fmt.Sprintf("Skill %s 已启用", id), "")
		return
	}
	s.redirectSettings(w, r, "skills", fmt.Sprintf("Skill %s 已禁用", id), "")
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

func displayTransport(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sse":
		return "sse"
	default:
		return "streamableHttp"
	}
}
