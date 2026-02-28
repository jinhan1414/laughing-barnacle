package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"laughing-barnacle/internal/llm"
)

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
		return "", errors.New(strings.TrimSpace(out))
	}
	return out, nil
}
