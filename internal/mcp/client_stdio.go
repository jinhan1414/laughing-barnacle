package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func (c *HTTPClient) callRPCStdio(ctx context.Context, service Service, method string, params map[string]any) (json.RawMessage, error) {
	command := strings.TrimSpace(service.Command)
	if command == "" {
		return nil, fmt.Errorf("stdio command is required")
	}

	cmd := exec.CommandContext(ctx, command, service.Args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdio stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdio stdout: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start stdio command: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	enc := json.NewEncoder(stdin)
	dec := json.NewDecoder(bufio.NewReader(stdout))

	initID := c.nextReqID()
	if err := enc.Encode(rpcRequest{
		JSONRPC: "2.0",
		ID:      initID,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": c.protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"clientInfo": map[string]any{
				"name":    "laughing-barnacle-agent",
				"version": "1.0.0",
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("write initialize request: %w", err)
	}
	initResp, err := waitRPCResponseFromSTDIO(dec, initID)
	if err != nil {
		if tail := strings.TrimSpace(stderr.String()); tail != "" {
			return nil, fmt.Errorf("read initialize response: %w; stderr: %s", err, tail)
		}
		return nil, fmt.Errorf("read initialize response: %w", err)
	}
	if initResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", initResp.Error.Code, initResp.Error.Message)
	}

	if err := enc.Encode(rpcRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		Params:  map[string]any{},
	}); err != nil {
		return nil, fmt.Errorf("write initialized notification: %w", err)
	}

	reqID := c.nextReqID()
	if err := enc.Encode(rpcRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  method,
		Params:  params,
	}); err != nil {
		return nil, fmt.Errorf("write rpc request: %w", err)
	}

	resp, err := waitRPCResponseFromSTDIO(dec, reqID)
	if err != nil {
		if tail := strings.TrimSpace(stderr.String()); tail != "" {
			return nil, fmt.Errorf("read rpc response: %w; stderr: %s", err, tail)
		}
		return nil, fmt.Errorf("read rpc response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}
