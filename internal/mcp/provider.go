package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"laughing-barnacle/internal/llm"
)

type ServiceStatus struct {
	Service   Service
	Connected bool
	ToolCount int
	Tools     []ServiceToolStatus
	Error     string
}

type ServiceToolStatus struct {
	Name        string
	Description string
	Enabled     bool
}

type ToolProvider struct {
	store  *Store
	client *HTTPClient

	cacheTTL time.Duration

	mu         sync.Mutex
	cacheUntil time.Time
	tools      []llm.ToolDefinition
	bindings   map[string]toolBinding
}

type toolBinding struct {
	ServiceID string
	ToolName  string
}

func NewToolProvider(store *Store, client *HTTPClient, cacheTTL time.Duration) *ToolProvider {
	if cacheTTL <= 0 {
		cacheTTL = 30 * time.Second
	}
	return &ToolProvider{
		store:    store,
		client:   client,
		cacheTTL: cacheTTL,
		bindings: make(map[string]toolBinding),
	}
}

func (p *ToolProvider) ListTools(ctx context.Context) ([]llm.ToolDefinition, error) {
	p.mu.Lock()
	if time.Now().Before(p.cacheUntil) && len(p.tools) > 0 {
		cached := cloneToolDefs(p.tools)
		p.mu.Unlock()
		return cached, nil
	}
	p.mu.Unlock()

	return p.RefreshTools(ctx)
}

func (p *ToolProvider) RefreshTools(ctx context.Context) ([]llm.ToolDefinition, error) {
	services := p.store.ListEnabledServices()
	defs := make([]llm.ToolDefinition, 0)
	bindings := make(map[string]toolBinding)

	for _, svc := range services {
		tools, err := p.client.ListTools(ctx, svc)
		if err != nil {
			continue
		}
		for _, tool := range tools {
			if !p.store.IsServiceToolEnabled(svc.ID, tool.Name) {
				continue
			}
			def, binding := toToolDefinition(svc, tool)
			name := def.Function.Name
			for i := 2; bindingExists(bindings, name); i++ {
				name = fmt.Sprintf("%s_%d", def.Function.Name, i)
			}
			def.Function.Name = name
			bindings[name] = binding
			defs = append(defs, def)
		}
	}

	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Function.Name < defs[j].Function.Name
	})

	p.mu.Lock()
	p.tools = defs
	p.bindings = bindings
	p.cacheUntil = time.Now().Add(p.cacheTTL)
	cached := cloneToolDefs(defs)
	p.mu.Unlock()

	return cached, nil
}

func (p *ToolProvider) CallTool(ctx context.Context, call llm.ToolCall) (string, error) {
	binding, ok := p.lookupBinding(call.Function.Name)
	if !ok {
		if _, err := p.RefreshTools(ctx); err != nil {
			return "", fmt.Errorf("refresh tools: %w", err)
		}
		binding, ok = p.lookupBinding(call.Function.Name)
		if !ok {
			return "", fmt.Errorf("unknown tool %q", call.Function.Name)
		}
	}

	service, exists := p.store.GetService(binding.ServiceID)
	if !exists {
		return "", fmt.Errorf("mcp service %q not found", binding.ServiceID)
	}
	if !service.Enabled {
		return "", fmt.Errorf("mcp service %q is disabled", binding.ServiceID)
	}
	if !p.store.IsServiceToolEnabled(binding.ServiceID, binding.ToolName) {
		return "", fmt.Errorf("mcp service %q tool %q is disabled", binding.ServiceID, binding.ToolName)
	}

	args, err := parseToolArguments(call.Function.Arguments)
	if err != nil {
		return "", fmt.Errorf("invalid tool arguments for %q: %w", call.Function.Name, err)
	}

	result, err := p.client.CallTool(ctx, service, binding.ToolName, args)
	if err != nil {
		return "", err
	}

	out := renderToolResult(result)
	if result.IsError {
		return "", fmt.Errorf(strings.TrimSpace(out))
	}
	return out, nil
}

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

func toToolDefinition(service Service, tool Tool) (llm.ToolDefinition, toolBinding) {
	prefix := sanitizeName(service.ID)
	toolName := sanitizeName(tool.Name)
	fullName := prefix + "__" + toolName
	if prefix == "" {
		fullName = toolName
	}

	description := strings.TrimSpace(tool.Description)
	if description == "" {
		description = "MCP tool"
	}
	description = fmt.Sprintf("[MCP %s] %s", service.Name, description)

	params := tool.InputSchema
	if params == nil {
		params = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	return llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionDefinition{
				Name:        fullName,
				Description: description,
				Parameters:  params,
			},
		}, toolBinding{
			ServiceID: service.ID,
			ToolName:  tool.Name,
		}
}

func sanitizeName(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "tool"
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func parseToolArguments(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}, nil
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

func renderToolResult(result ToolCallResult) string {
	textParts := make([]string, 0, len(result.Content))
	for _, item := range result.Content {
		if strings.EqualFold(item.Type, "text") && strings.TrimSpace(item.Text) != "" {
			textParts = append(textParts, item.Text)
		}
	}
	if len(textParts) > 0 {
		return strings.Join(textParts, "\n")
	}

	if result.StructuredContent != nil {
		data, err := json.Marshal(result.StructuredContent)
		if err == nil {
			return string(data)
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func bindingExists(bindings map[string]toolBinding, name string) bool {
	_, ok := bindings[name]
	return ok
}

func cloneToolDefs(defs []llm.ToolDefinition) []llm.ToolDefinition {
	out := make([]llm.ToolDefinition, len(defs))
	copy(out, defs)
	return out
}
