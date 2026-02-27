package a2a

import (
	"fmt"
	"laughing-barnacle/internal/mcp"
	"net/http"
	"strings"
	"time"
)

const defaultRequestTimeout = 20 * time.Second

type Provider struct {
	store *mcp.Store
	http  *http.Client
}

func NewProvider(store *mcp.Store, timeout time.Duration) *Provider {
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	transport := &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		DisableKeepAlives: true,
	}
	return &Provider{
		store: store,
		http: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

func (p *Provider) ListIndexLines(limit int) []string {
	if p == nil || p.store == nil {
		return nil
	}
	agents := p.store.ListEnabledA2AAgents()
	if limit > 0 && len(agents) > limit {
		agents = agents[:limit]
	}
	out := make([]string, 0, len(agents))
	for _, item := range agents {
		line := fmt.Sprintf(
			"agent_id=%s | name=%s | description=%s | status=enabled",
			strings.TrimSpace(item.ID),
			strings.TrimSpace(item.Name),
			trimText(item.Description, 40),
		)
		out = append(out, line)
	}
	return out
}

func trimText(raw string, max int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || max <= 0 {
		return ""
	}
	runes := []rune(raw)
	if len(runes) <= max {
		return raw
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return strings.TrimSpace(string(runes[:max-3])) + "..."
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}
	return ""
}
