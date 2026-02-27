package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/mcp"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	maxStatusMethodAttempts = 3
	retryBaseDelay          = 150 * time.Millisecond
)

func (p *Provider) Send(ctx context.Context, req agent.A2ASendRequest) (agent.A2ATaskResult, error) {
	target, err := p.loadEnabledAgent(req.AgentID)
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	params := buildSendParams(req)
	result, err := p.callRPC(ctx, target, "message/send", params)
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	return parseTaskResult(req.AgentID, "", result), nil
}

func (p *Provider) GetTask(ctx context.Context, req agent.A2ATaskQuery) (agent.A2ATaskResult, error) {
	target, err := p.loadEnabledAgent(req.AgentID)
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	result, err := p.callRPC(ctx, target, "tasks/get", map[string]any{"id": req.TaskID})
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	return parseTaskResult(req.AgentID, req.TaskID, result), nil
}

func (p *Provider) CancelTask(ctx context.Context, req agent.A2ATaskQuery) (agent.A2ATaskResult, error) {
	target, err := p.loadEnabledAgent(req.AgentID)
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	result, err := p.callRPC(ctx, target, "tasks/cancel", map[string]any{"id": req.TaskID})
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	return parseTaskResult(req.AgentID, req.TaskID, result), nil
}

func (p *Provider) loadEnabledAgent(agentID string) (mcp.A2AAgent, error) {
	if p == nil || p.store == nil {
		return mcp.A2AAgent{}, fmt.Errorf("a2a registry unavailable")
	}
	record, ok := p.store.GetA2AAgent(strings.TrimSpace(agentID))
	if !ok || !record.Enabled {
		return mcp.A2AAgent{}, fmt.Errorf("agent not found or disabled: %s", strings.TrimSpace(agentID))
	}
	return record, nil
}

func buildSendParams(req agent.A2ASendRequest) map[string]any {
	params := map[string]any{
		"message": map[string]any{
			"role": "user",
			"parts": []map[string]any{
				{"type": "text", "text": strings.TrimSpace(req.Message)},
			},
		},
	}
	if text := strings.TrimSpace(req.SessionID); text != "" {
		params["sessionId"] = text
	}
	if len(req.Metadata) > 0 {
		params["metadata"] = req.Metadata
	}
	return params
}

func (p *Provider) callRPC(ctx context.Context, target mcp.A2AAgent, method string, params map[string]any) (map[string]any, error) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      strconv.FormatInt(time.Now().UnixNano(), 10),
		"method":  method,
		"params":  params,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode a2a request: %w", err)
	}
	maxAttempts := methodAttemptLimit(method)
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, callErr := p.callRPCOnce(ctx, target, method, body)
		if callErr == nil {
			return result, nil
		}
		lastErr = callErr
		if !shouldRetryRPCError(callErr) || attempt >= maxAttempts {
			break
		}
		if err := waitRetryBackoff(ctx, attempt); err != nil {
			return nil, err
		}
	}
	if maxAttempts > 1 {
		return nil, fmt.Errorf("%w (attempts=%d)", lastErr, maxAttempts)
	}
	return nil, lastErr
}

func (p *Provider) callRPCOnce(ctx context.Context, target mcp.A2AAgent, method string, body []byte) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build a2a request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(target.AuthToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call a2a method %s: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("call a2a method %s: status=%d", method, resp.StatusCode)
	}
	var envelope rpcEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode a2a response: %w", err)
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("a2a rpc error (%d): %s", envelope.Error.Code, strings.TrimSpace(envelope.Error.Message))
	}
	if len(envelope.Result) == 0 {
		return nil, fmt.Errorf("a2a response missing result")
	}
	var result map[string]any
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return nil, fmt.Errorf("decode a2a result: %w", err)
	}
	return result, nil
}

func methodAttemptLimit(method string) int {
	if strings.EqualFold(strings.TrimSpace(method), "message/send") {
		return 1
	}
	return maxStatusMethodAttempts
}

func shouldRetryRPCError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "use of closed network connection")
}

func waitRetryBackoff(ctx context.Context, attempt int) error {
	delay := retryBaseDelay * time.Duration(attempt)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("a2a retry canceled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func parseTaskResult(agentID, fallbackTaskID string, result map[string]any) agent.A2ATaskResult {
	task := readMap(result, "task")
	if len(task) == 0 {
		task = result
	}
	taskID := firstNonEmpty(
		readString(task, "id"),
		readString(result, "id"),
		strings.TrimSpace(fallbackTaskID),
	)
	status := firstNonEmpty(
		readNestedString(task, "status", "state"),
		readString(task, "status"),
		readNestedString(result, "status", "state"),
		readString(result, "status"),
		"unknown",
	)
	message := firstNonEmpty(
		readString(task, "message"),
		readString(result, "message"),
	)
	artifacts := extractArtifacts(task)
	if len(artifacts) == 0 {
		artifacts = extractArtifacts(result)
	}
	return agent.A2ATaskResult{
		AgentID:   strings.TrimSpace(agentID),
		TaskID:    taskID,
		Status:    status,
		Message:   message,
		Artifacts: artifacts,
	}
}

func extractArtifacts(payload map[string]any) []string {
	items, ok := payload["artifacts"].([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		part, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text := firstNonEmpty(readString(part, "text"), readString(part, "value"), readString(part, "id"))
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func readNestedString(m map[string]any, key, nested string) string {
	child := readMap(m, key)
	return readString(child, nested)
}

func readMap(m map[string]any, key string) map[string]any {
	if len(m) == 0 {
		return nil
	}
	value, ok := m[key]
	if !ok {
		return nil
	}
	child, _ := value.(map[string]any)
	return child
}

func readString(m map[string]any, key string) string {
	if len(m) == 0 {
		return ""
	}
	value, ok := m[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
