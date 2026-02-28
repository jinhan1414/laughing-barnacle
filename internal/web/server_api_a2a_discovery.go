package web

import (
	"context"
	"fmt"
	"laughing-barnacle/internal/mcp"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
)

const a2aAgentCardResolveTimeout = 10 * time.Second

type discoveredA2AAgent struct {
	Name            string
	Description     string
	Endpoint        string
	ProtocolVersion string
	Skills          []mcp.A2ASkill
}

func discoverA2AAgentFromCard(ctx context.Context, cardURL, authToken string) (discoveredA2AAgent, error) {
	baseURL, opts, err := buildAgentCardResolveTarget(cardURL, authToken)
	if err != nil {
		return discoveredA2AAgent{}, err
	}
	resolver := agentcard.NewResolver(&http.Client{Timeout: a2aAgentCardResolveTimeout})
	card, err := resolver.Resolve(ctx, baseURL, opts...)
	if err != nil {
		return discoveredA2AAgent{}, fmt.Errorf("resolve agent card: %w", err)
	}
	endpoint := strings.TrimSpace(card.URL)
	if endpoint == "" {
		return discoveredA2AAgent{}, fmt.Errorf("agent card missing url field")
	}
	skills := mapAgentSkills(card.Skills)
	if len(skills) == 0 {
		return discoveredA2AAgent{}, fmt.Errorf("agent card skills is required")
	}
	return discoveredA2AAgent{
		Name:            strings.TrimSpace(card.Name),
		Description:     strings.TrimSpace(card.Description),
		Endpoint:        endpoint,
		ProtocolVersion: strings.TrimSpace(card.ProtocolVersion),
		Skills:          skills,
	}, nil
}

func buildAgentCardResolveTarget(cardURL, authToken string) (string, []agentcard.ResolveOption, error) {
	raw := strings.TrimSpace(cardURL)
	if raw == "" {
		return "", nil, fmt.Errorf("agent card url is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", nil, fmt.Errorf("parse agent card url: %w", err)
	}
	if strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return "", nil, fmt.Errorf("agent card url must include scheme and host")
	}
	baseURL := parsed.Scheme + "://" + parsed.Host
	opts := make([]agentcard.ResolveOption, 0, 2)
	pathWithQuery := strings.TrimSpace(parsed.EscapedPath())
	if strings.TrimSpace(parsed.RawQuery) != "" {
		pathWithQuery = pathWithQuery + "?" + strings.TrimSpace(parsed.RawQuery)
	}
	if pathWithQuery != "" && pathWithQuery != "/" && pathWithQuery != "/.well-known/agent-card.json" {
		opts = append(opts, agentcard.WithPath(pathWithQuery))
	}
	if token := strings.TrimSpace(authToken); token != "" {
		opts = append(opts, agentcard.WithRequestHeader("Authorization", "Bearer "+token))
	}
	return baseURL, opts, nil
}

func mapAgentSkills(skills []a2a.AgentSkill) []mcp.A2ASkill {
	if len(skills) == 0 {
		return nil
	}
	out := make([]mcp.A2ASkill, 0, len(skills))
	for _, skill := range skills {
		id := strings.TrimSpace(skill.ID)
		name := strings.TrimSpace(skill.Name)
		desc := strings.TrimSpace(skill.Description)
		if id == "" && name == "" && desc == "" {
			continue
		}
		if id == "" {
			id = name
		}
		if name == "" {
			name = id
		}
		out = append(out, mcp.A2ASkill{ID: id, Name: name, Description: desc})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func applyDiscoveredA2AAgentFields(req *apiA2ASaveRequest, discovered discoveredA2AAgent) {
	req.Endpoint = strings.TrimSpace(discovered.Endpoint)
	req.ProtocolVersion = strings.TrimSpace(discovered.ProtocolVersion)
	req.Skills = append([]mcp.A2ASkill(nil), discovered.Skills...)
	if strings.TrimSpace(req.Name) == "" {
		req.Name = strings.TrimSpace(discovered.Name)
	}
	if strings.TrimSpace(req.Description) == "" {
		req.Description = strings.TrimSpace(discovered.Description)
	}
}
