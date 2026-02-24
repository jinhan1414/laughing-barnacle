package mcp

import (
	"context"
	"fmt"
	"sort"

	"laughing-barnacle/internal/llm"
	"sync"
	"time"
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
