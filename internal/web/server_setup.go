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
	var turnManager *chatTurnManager
	if convStore != nil && agent != nil {
		turnManager, err = newChatTurnManager(convStore, agent)
		if err != nil {
			return nil, err
		}
	}

	return &Server{
		agent:      agent,
		turns:      turnManager,
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
	mux.HandleFunc("/settings/a2a/save", s.handleSettingsA2ASave)
	mux.HandleFunc("/settings/a2a/delete", s.handleSettingsA2ADelete)
	mux.HandleFunc("/settings/a2a/toggle", s.handleSettingsA2AToggle)
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
	mux.HandleFunc("/api/mcp/services/save", s.handleAPIMCPServiceSave)
	mux.HandleFunc("/api/mcp/services/toggle", s.handleAPIMCPServiceToggle)
	mux.HandleFunc("/api/mcp/services/delete", s.handleAPIMCPServiceDelete)
	mux.HandleFunc("/api/a2a/agents", s.handleAPIA2AAgents)
	mux.HandleFunc("/api/a2a/agents/read", s.handleAPIA2AAgentRead)
	mux.HandleFunc("/api/a2a/agents/save", s.handleAPIA2AAgentSave)
	mux.HandleFunc("/api/a2a/agents/toggle", s.handleAPIA2AAgentToggle)
	mux.HandleFunc("/api/a2a/agents/delete", s.handleAPIA2AAgentDelete)
	mux.HandleFunc("/api/skills", s.handleAPISkills)
	mux.HandleFunc("/api/skills/index", s.handleAPISkillIndex)
	mux.HandleFunc("/api/skills/read", s.handleAPISkillRead)
	mux.HandleFunc("/api/skills/save", s.handleAPISkillSave)
	mux.HandleFunc("/api/skills/toggle", s.handleAPISkillToggle)
	mux.HandleFunc("/api/skills/delete", s.handleAPISkillDelete)
	mux.HandleFunc("/api/skills/install", s.handleAPISkillInstall)
	mux.HandleFunc("/api/skills/catalog/search", s.handleAPISkillsCatalogSearch)
	mux.HandleFunc("/api/schedules", s.handleAPISchedules)
	mux.HandleFunc("/api/schedules/save", s.handleAPIScheduleSave)
	mux.HandleFunc("/api/schedules/toggle", s.handleAPIScheduleToggle)
	mux.HandleFunc("/api/schedules/delete", s.handleAPIScheduleDelete)
	mux.HandleFunc("/api/schedules/run", s.handleAPIScheduleRun)
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
	mux.HandleFunc("/api/chat/send", s.handleAPIChatSend)
	mux.HandleFunc("/api/chat/stream", s.handleAPIChatStream)
	mux.HandleFunc("/api/chat/updates", s.handleAPIChatUpdates)
	mux.HandleFunc("/api/chat/archive/index", s.handleAPIChatArchiveIndex)
	mux.HandleFunc("/api/chat/archive/section", s.handleAPIChatArchiveSection)
	mux.HandleFunc("/api/chat/tools/run", s.handleAPIChatToolRun)
	mux.HandleFunc("/healthz", s.handleHealthz)
}
