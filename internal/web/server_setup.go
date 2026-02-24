package web

import (
	"html/template"
	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llmlog"
	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/memory"
	"laughing-barnacle/internal/skills"
	"net/http"
	"time"
)

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

func (s *Server) SetMemoryStore(store *memory.Store) {
	s.memoryStore = store
}

func (s *Server) SetMemoryWorkerConfig(interval, idleWindow, maxWindow time.Duration, maxMessages int) {
	if interval > 0 {
		s.Interval = interval
	}
	if idleWindow > 0 {
		s.IdleWindow = idleWindow
	}
	if maxWindow > 0 {
		s.MaxWindow = maxWindow
	}
	if maxMessages > 0 {
		s.MaxMessages = maxMessages
	}
}

func (s *Server) memoryWorkerConfigOrDefault() memoryWorkerConfig {
	cfg := s.memoryWorkerConfig
	if cfg.Interval <= 0 {
		cfg.Interval = defaultMemoryWorkerInterval
	}
	if cfg.IdleWindow <= 0 {
		cfg.IdleWindow = defaultMemoryIdleWindow
	}
	if cfg.MaxWindow <= 0 {
		cfg.MaxWindow = defaultMemoryMaxSegmentWindow
	}
	if cfg.MaxMessages <= 0 {
		cfg.MaxMessages = defaultMemoryMaxSegmentMessage
	}
	return cfg
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
	mux.HandleFunc("/settings/schedules/delete", s.handleSettingsScheduleDelete)
	mux.HandleFunc("/settings/schedules/toggle", s.handleSettingsScheduleToggle)
	mux.HandleFunc("/settings/schedules/run", s.handleSettingsScheduleRun)
	mux.HandleFunc("/settings/memory/inbox/review", s.handleSettingsMemoryInboxReview)
	mux.HandleFunc("/settings/memory/maintenance/run", s.handleSettingsMemoryMaintenanceRun)
	mux.HandleFunc("/settings/llm/prompts/save", s.handleSettingsLLMPromptsSave)
	mux.HandleFunc("/settings/llm/prompts/reset", s.handleSettingsLLMPromptsReset)
	mux.HandleFunc("/api/mcp/services", s.handleAPIMCPServices)
	mux.HandleFunc("/api/skills", s.handleAPISkills)
	mux.HandleFunc("/api/skills/read", s.handleAPISkillRead)
	mux.HandleFunc("/api/skills/catalog/search", s.handleAPISkillsCatalogSearch)
	mux.HandleFunc("/api/schedules", s.handleAPISchedules)
	mux.HandleFunc("/api/memory/index", s.handleAPIMemoryIndex)
	mux.HandleFunc("/api/memory/read", s.handleAPIMemoryRead)
	mux.HandleFunc("/api/memory/section", s.handleAPIMemorySection)
	mux.HandleFunc("/api/memory/upsert", s.handleAPIMemoryUpsert)
	mux.HandleFunc("/api/memory/move", s.handleAPIMemoryMove)
	mux.HandleFunc("/api/memory/delete", s.handleAPIMemoryDelete)
	mux.HandleFunc("/api/memory/inbox", s.handleAPIMemoryInbox)
	mux.HandleFunc("/api/memory/inbox/review", s.handleAPIMemoryInboxReview)
	mux.HandleFunc("/api/memory/maintenance/run", s.handleAPIMemoryMaintenanceRun)
	mux.HandleFunc("/api/memory/rollback", s.handleAPIMemoryRollback)
	mux.HandleFunc("/api/memory/audit", s.handleAPIMemoryAudit)
	mux.HandleFunc("/api/memory/metrics", s.handleAPIMemoryMetrics)
	mux.HandleFunc("/api/chat/updates", s.handleAPIChatUpdates)
	mux.HandleFunc("/api/chat/tools/run", s.handleAPIChatToolRun)
	mux.HandleFunc("/healthz", s.handleHealthz)
}
