package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

func (c *HTTPClient) callRPCStdio(ctx context.Context, service Service, method string, params map[string]any) (json.RawMessage, error) {
	session, err := c.ensureStdioSession(ctx, service)
	if err != nil {
		return nil, err
	}

	result, err := session.call(ctx, c.nextReqID(), method, params)
	if err == nil {
		return result, nil
	}

	c.invalidateStdioSession(service.ID, session)
	if ctx.Err() != nil {
		return nil, err
	}

	recreated, recreateErr := c.ensureStdioSession(ctx, service)
	if recreateErr != nil {
		return nil, fmt.Errorf("rpc failed: %v; recreate stdio session failed: %w", err, recreateErr)
	}
	result, retryErr := recreated.call(ctx, c.nextReqID(), method, params)
	if retryErr != nil {
		c.invalidateStdioSession(service.ID, recreated)
		return nil, fmt.Errorf("rpc failed after stdio session retry: %w", retryErr)
	}
	return result, nil
}
