package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"laughing-barnacle/internal/agent"
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
	return &Provider{
		store: store,
		http:  &http.Client{Timeout: timeout},
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

func (p *Provider) ReadAgentDetail(agentID string) (agent.A2AAgentDetail, bool) {
	if p == nil || p.store == nil {
		return agent.A2AAgentDetail{}, false
	}
	record, ok := p.store.GetA2AAgent(strings.TrimSpace(agentID))
	if !ok {
		return agent.A2AAgentDetail{}, false
	}
	return toAgentDetail(record), true
}

func (p *Provider) Register(ctx context.Context, req agent.A2ARegisterRequest) (agent.A2AAgentDetail, error) {
	input, err := normalizeRegisterInput(ctx, p.http, req)
	if err != nil {
		return agent.A2AAgentDetail{}, err
	}
	if p == nil || p.store == nil {
		return agent.A2AAgentDetail{}, fmt.Errorf("a2a registry unavailable")
	}
	if err := p.store.UpsertA2AAgent(input); err != nil {
		return agent.A2AAgentDetail{}, err
	}
	record, ok := p.findBySignature(input)
	if !ok {
		return agent.A2AAgentDetail{}, fmt.Errorf("register a2a agent succeeded but lookup failed")
	}
	return toAgentDetail(record), nil
}

func (p *Provider) findBySignature(input mcp.A2AAgent) (mcp.A2AAgent, bool) {
	agents := p.store.ListA2AAgents()
	endpoint := strings.TrimSpace(input.Endpoint)
	cardURL := strings.TrimSpace(input.AgentCardURL)
	for _, item := range agents {
		if endpoint != "" && strings.EqualFold(strings.TrimSpace(item.Endpoint), endpoint) {
			return item, true
		}
		if cardURL != "" && strings.EqualFold(strings.TrimSpace(item.AgentCardURL), cardURL) {
			return item, true
		}
	}
	return mcp.A2AAgent{}, false
}

func normalizeRegisterInput(ctx context.Context, client *http.Client, req agent.A2ARegisterRequest) (mcp.A2AAgent, error) {
	req.AgentCardURL = strings.TrimSpace(req.AgentCardURL)
	req.AgentCardJSON = strings.TrimSpace(req.AgentCardJSON)
	req.Alias = strings.TrimSpace(req.Alias)
	req.Description = strings.TrimSpace(req.Description)
	req.Endpoint = strings.TrimSpace(req.Endpoint)
	req.AuthToken = strings.TrimSpace(req.AuthToken)

	card, err := readAgentCard(ctx, client, req.AgentCardURL, req.AgentCardJSON)
	if err != nil {
		return mcp.A2AAgent{}, err
	}

	name := firstNonEmpty(req.Alias, readCardString(card, "name"), readCardString(card, "title"))
	endpoint := firstNonEmpty(req.Endpoint, readCardString(card, "url"), readCardString(card, "endpoint"), readCardString(card, "base_url"))
	description := firstNonEmpty(req.Description, readCardString(card, "description"))

	if endpoint == "" {
		return mcp.A2AAgent{}, fmt.Errorf("a2a agent endpoint is required")
	}
	if name == "" {
		name = endpoint
	}
	return mcp.A2AAgent{
		Name:         name,
		Description:  description,
		Endpoint:     endpoint,
		AgentCardURL: req.AgentCardURL,
		AuthToken:    req.AuthToken,
		Enabled:      req.Enabled,
	}, nil
}

func readAgentCard(ctx context.Context, client *http.Client, cardURL, cardJSON string) (map[string]any, error) {
	if cardJSON != "" {
		return parseAgentCard(cardJSON)
	}
	if cardURL == "" {
		return map[string]any{}, nil
	}
	if client == nil {
		return nil, fmt.Errorf("a2a http client unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cardURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build agent card request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch agent card: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch agent card: status=%d", resp.StatusCode)
	}
	var card map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("decode agent card: %w", err)
	}
	return card, nil
}

func parseAgentCard(raw string) (map[string]any, error) {
	var card map[string]any
	if err := json.Unmarshal([]byte(raw), &card); err != nil {
		return nil, fmt.Errorf("decode agent_card_json: %w", err)
	}
	if card == nil {
		return nil, fmt.Errorf("agent card is empty")
	}
	return card, nil
}

func readCardString(card map[string]any, key string) string {
	value, ok := card[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
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

func toAgentDetail(item mcp.A2AAgent) agent.A2AAgentDetail {
	return agent.A2AAgentDetail{
		ID:           item.ID,
		Name:         item.Name,
		Description:  item.Description,
		Endpoint:     item.Endpoint,
		AgentCardURL: item.AgentCardURL,
		Enabled:      item.Enabled,
		UpdatedAt:    item.UpdatedAt,
	}
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
