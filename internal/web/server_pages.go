package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/memory"
)

func (s *Server) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	data := logsPageData{Entries: s.logStore.List()}
	_ = s.tmpl.ExecuteTemplate(w, "logs.html", data)
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	section := strings.TrimSpace(r.URL.Query().Get("section"))
	if section == "" {
		section = "mcp"
	}
	if section != "mcp" && section != "a2a" && section != "async_tasks" && section != "llm" && section != "security" && section != "skills" && section != "schedules" && section != "memory" {
		section = "mcp"
	}

	data := settingsPageData{
		ActiveSection: section,
		Sections: []settingsSection{
			{Key: "mcp", Title: "MCP 服务", Description: "管理 Agent 可用的 MCP 工具服务"},
			{Key: "a2a", Title: "A2A 接入", Description: "管理已接入的 A2A Agent 注册信息"},
			{Key: "async_tasks", Title: "后台任务", Description: "查看后台任务清单与全量执行日志"},
			{Key: "memory", Title: "记忆模块", Description: "可视化查看 MemoryFS 命名空间、节点与沉淀分段"},
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
				ID:         status.Service.ID,
				Name:       status.Service.Name,
				Endpoint:   status.Service.Endpoint,
				Command:    status.Service.Command,
				Args:       strings.Join(status.Service.Args, " "),
				EnvKeys:    joinSortedMapKeys(status.Service.Env),
				HeaderKeys: joinSortedMapKeys(status.Service.Headers),
				Transport:  displayTransport(status.Service.Transport),
				Enabled:    status.Service.Enabled,
				UpdatedAt:  status.Service.UpdatedAt.Format("2006-01-02 15:04:05"),
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
	} else if section == "a2a" {
		allAgents := s.mcpStore.ListA2AAgents()
		data.A2AAgents = make([]a2aAgentView, 0, len(allAgents))
		for _, item := range allAgents {
			view := a2aAgentView{
				ID:              item.ID,
				Name:            item.Name,
				Description:     item.Description,
				Endpoint:        item.Endpoint,
				AgentCardURL:    item.AgentCardURL,
				ProtocolVersion: item.ProtocolVersion,
				Skills:          append([]mcp.A2ASkill(nil), item.Skills...),
				HasAuthToken:    strings.TrimSpace(item.AuthToken) != "",
				Enabled:         item.Enabled,
			}
			if !item.UpdatedAt.IsZero() {
				view.UpdatedAt = item.UpdatedAt.Format("2006-01-02 15:04:05")
			}
			data.A2AAgents = append(data.A2AAgents, view)
		}
	} else if section == "async_tasks" {
		allTasks := s.agent.ListAsyncTasks()
		data.AsyncTasks = make([]asyncTaskView, 0, len(allTasks))
		for _, task := range allTasks {
			view := asyncTaskView{
				ID:                task.ID,
				TaskType:          task.TaskType,
				Status:            task.Status,
				TrackerState:      task.TrackerState,
				TrackerReason:     task.TrackerReason,
				Request:           task.Request,
				AgentID:           task.AgentID,
				RemoteTaskID:      task.RemoteTaskID,
				Result:            task.Result,
				Error:             task.Error,
				TrackingRenewals:  task.TrackingRenewals,
				ConsecutiveErrors: task.ConsecutiveErrors,
			}
			if !task.CreatedAt.IsZero() {
				view.CreatedAt = task.CreatedAt.Format("2006-01-02 15:04:05")
			}
			if !task.UpdatedAt.IsZero() {
				view.UpdatedAt = task.UpdatedAt.Format("2006-01-02 15:04:05")
			}
			if !task.NextPollAt.IsZero() {
				view.NextPollAt = task.NextPollAt.Format("2006-01-02 15:04:05")
			}
			if !task.LastRenewedAt.IsZero() {
				view.LastRenewedAt = task.LastRenewedAt.Format("2006-01-02 15:04:05")
			}
			if !task.LastReconciledAt.IsZero() {
				view.LastReconciledAt = task.LastReconciledAt.Format("2006-01-02 15:04:05")
			}
			view.Logs = make([]asyncTaskLogView, 0, len(task.Logs))
			for _, log := range task.Logs {
				item := asyncTaskLogView{
					Cursor:  log.Cursor,
					Level:   log.Level,
					Message: log.Message,
				}
				if !log.CreatedAt.IsZero() {
					item.CreatedAt = log.CreatedAt.Format("2006-01-02 15:04:05")
				}
				view.Logs = append(view.Logs, item)
			}
			data.AsyncTasks = append(data.AsyncTasks, view)
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
	} else if section == "memory" {
		if s.memoryStore != nil {
			metrics := s.memoryStore.GetMetrics()
			data.MemoryMetrics = memoryMetricsView{
				SegmentTotal:      metrics.SegmentTotal,
				SegmentOpen:       metrics.SegmentOpen,
				SegmentClosed:     metrics.SegmentClosed,
				SegmentProcessing: metrics.SegmentProcessing,
				SegmentPersisted:  metrics.SegmentPersisted,
				SegmentFailed:     metrics.SegmentFailed,
				FailedRate:        fmt.Sprintf("%.1f%%", metrics.FailedRate*100),
				RetryTotal:        metrics.RetryTotal,
				PendingCount:      metrics.PendingCount,
				ReviewedCount:     metrics.ReviewedCount,
				WarningFailRate:   metrics.WarningFailRate,
				WarningPending:    metrics.WarningPending,
				WarningRetry:      metrics.WarningRetry,
			}
			if !metrics.LastPersistedAt.IsZero() {
				data.MemoryMetrics.LastPersistedAt = metrics.LastPersistedAt.Format("2006-01-02 15:04:05")
			}

			allNodes := s.memoryStore.ListNodes(300)
			data.MemoryNodes = make([]memoryNodeView, 0, len(allNodes))
			for _, item := range allNodes {
				view := memoryNodeView{
					Path:       item.Path,
					Title:      item.Title,
					Type:       string(item.Type),
					SchemaKind: item.SchemaKind,
					Revision:   item.Revision,
				}
				if item.Type == memory.NodeTypeFile && item.Content != nil {
					view.Summary = item.Content.Summary
				}
				if !item.UpdatedAt.IsZero() {
					view.UpdatedAt = item.UpdatedAt.Format("2006-01-02 15:04:05")
				}
				data.MemoryNodes = append(data.MemoryNodes, view)
			}

			segments := s.memoryStore.ListSegments(80)
			data.MemorySegments = make([]memorySegmentView, 0, len(segments))
			for _, seg := range segments {
				view := memorySegmentView{
					ID:             seg.ID,
					Status:         string(seg.Status),
					RetryCount:     seg.RetryCount,
					Turns:          len(seg.Turns),
					CloseReason:    seg.CloseReason,
					PersistedPaths: append([]string(nil), seg.PersistedPaths...),
					Error:          seg.Error,
				}
				if !seg.LastUserAt.IsZero() {
					view.LastUserAt = seg.LastUserAt.Format("2006-01-02 15:04:05")
				}
				if !seg.UpdatedAt.IsZero() {
					view.UpdatedAt = seg.UpdatedAt.Format("2006-01-02 15:04:05")
				}
				data.MemorySegments = append(data.MemorySegments, view)
			}

			pending := s.memoryStore.ListInboxPending(120)
			data.MemoryPending = make([]memoryPendingView, 0, len(pending))
			for _, node := range pending {
				view := memoryPendingView{
					Path:    node.Path,
					Title:   node.Title,
					Summary: "",
				}
				if node.Content != nil {
					view.Summary = node.Content.Summary
					view.TargetPath = findMemoryRefValue(node.Content.Refs, "target_path")
					if raw := strings.TrimSpace(findMemoryRefValue(node.Content.Refs, "target_confidence")); raw != "" {
						view.Confidence = raw
					}
				}
				if !node.UpdatedAt.IsZero() {
					view.UpdatedAt = node.UpdatedAt.Format("2006-01-02 15:04:05")
				}
				data.MemoryPending = append(data.MemoryPending, view)
			}
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
		if s.memoryStore != nil {
			policy := s.memoryWorkerConfigOrDefault()
			metrics := s.memoryStore.GetMetrics()
			data.MemoryTask = scheduleMemoryMaintenanceView{
				Available:        true,
				Driver:           "memory_worker",
				Interval:         policy.Interval.String(),
				IdleWindow:       policy.IdleWindow.String(),
				MaxWindow:        policy.MaxWindow.String(),
				MaxMessages:      policy.MaxMessages,
				SegmentTotal:     metrics.SegmentTotal,
				SegmentPersisted: metrics.SegmentPersisted,
				SegmentFailed:    metrics.SegmentFailed,
				FailedRate:       fmt.Sprintf("%.1f%%", metrics.FailedRate*100),
				RetryTotal:       metrics.RetryTotal,
				PendingCount:     metrics.PendingCount,
				WarningFailRate:  metrics.WarningFailRate,
				WarningPending:   metrics.WarningPending,
				WarningRetry:     metrics.WarningRetry,
			}
			if maintenanceAudit, ok := latestMaintenanceAudit(s.memoryStore.ListAudits(200)); ok {
				if !maintenanceAudit.CreatedAt.IsZero() {
					data.MemoryTask.LastRunAt = maintenanceAudit.CreatedAt.Format("2006-01-02 15:04:05")
				}
				data.MemoryTask.LastRunDetail = strings.TrimSpace(maintenanceAudit.Detail)
			}
			if !metrics.LastPersistedAt.IsZero() {
				data.MemoryTask.LastPersistedAt = metrics.LastPersistedAt.Format("2006-01-02 15:04:05")
			}
		}
	}

	_ = s.tmpl.ExecuteTemplate(w, "settings.html", data)
}
