package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llmlog"
	"laughing-barnacle/internal/mcp"
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
	tmpl       *template.Template
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

func NewServer(
	agent *agent.Agent,
	convStore *conversation.Store,
	logStore *llmlog.Store,
	mcpStore *mcp.Store,
	mcpTools *mcp.ToolProvider,
	skillStore *skills.Store,
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
		tmpl:       tmpl,
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
	mux.HandleFunc("/settings/mcp/tool/toggle", s.handleSettingsMCPToolToggle)
	mux.HandleFunc("/settings/skills/install", s.handleSettingsSkillInstall)
	mux.HandleFunc("/settings/skills/save", s.handleSettingsSkillSave)
	mux.HandleFunc("/settings/skills/delete", s.handleSettingsSkillDelete)
	mux.HandleFunc("/settings/skills/toggle", s.handleSettingsSkillToggle)
	mux.HandleFunc("/settings/llm/prompts/save", s.handleSettingsLLMPromptsSave)
	mux.HandleFunc("/settings/llm/prompts/reset", s.handleSettingsLLMPromptsReset)
	mux.HandleFunc("/api/mcp/services", s.handleAPIMCPServices)
	mux.HandleFunc("/api/skills", s.handleAPISkills)
	mux.HandleFunc("/api/skills/catalog/search", s.handleAPISkillsCatalogSearch)
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
	case "stdio":
		return "stdio"
	default:
		return "streamableHttp"
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
