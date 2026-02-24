package mcp

import (
	"context"
	"sort"
	"strings"
	"time"
)

func (p *ToolProvider) ListServiceStatuses(ctx context.Context) []ServiceStatus {
	services := p.store.ListServices()
	statuses := make([]ServiceStatus, 0, len(services))

	for _, svc := range services {
		if !svc.Enabled {
			statuses = append(statuses, ServiceStatus{
				Service:   svc,
				Connected: false,
				ToolCount: 0,
				Error:     "未启用",
			})
			continue
		}

		tools, err := p.client.ListTools(ctx, svc)
		if err != nil {
			statuses = append(statuses, ServiceStatus{
				Service:   svc,
				Connected: false,
				ToolCount: 0,
				Error:     err.Error(),
			})
			continue
		}

		toolStatuses := make([]ServiceToolStatus, 0, len(tools))
		enabledCount := 0
		for _, tool := range tools {
			enabled := p.store.IsServiceToolEnabled(svc.ID, tool.Name)
			if enabled {
				enabledCount++
			}
			toolStatuses = append(toolStatuses, ServiceToolStatus{
				Name:        tool.Name,
				Description: strings.TrimSpace(tool.Description),
				Enabled:     enabled,
			})
		}
		sort.Slice(toolStatuses, func(i, j int) bool {
			return toolStatuses[i].Name < toolStatuses[j].Name
		})

		statuses = append(statuses, ServiceStatus{
			Service:   svc,
			Connected: true,
			ToolCount: enabledCount,
			Tools:     toolStatuses,
		})
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Service.ID < statuses[j].Service.ID
	})
	return statuses
}

func (p *ToolProvider) InvalidateCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cacheUntil = time.Time{}
}

func (p *ToolProvider) lookupBinding(toolName string) (toolBinding, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	binding, ok := p.bindings[toolName]
	return binding, ok
}
