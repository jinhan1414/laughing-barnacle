package a2a

import (
	"context"
	"errors"
	"fmt"
	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/mcp"
	"net"
	"net/url"
	"strings"
	"time"

	a2asdk "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
)

const (
	maxStatusMethodAttempts = 3
	retryBaseDelay          = 150 * time.Millisecond
	sdkProviderName         = "a2a-go"
	sdkMethodSendMessage    = "SendMessage"
	sdkMethodGetTask        = "GetTask"
	sdkMethodCancelTask     = "CancelTask"
)

func (p *Provider) Send(ctx context.Context, req agent.A2ASendRequest) (agent.A2ATaskResult, error) {
	target, err := p.loadEnabledAgent(req.AgentID)
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	client, err := p.buildSDKClient(ctx, target, false)
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	params := buildSendMessageParams(req)
	var result a2asdk.SendMessageResult
	if err := p.callWithRetry(ctx, sdkMethodSendMessage, func() error {
		resp, callErr := client.SendMessage(ctx, params)
		if callErr != nil {
			return callErr
		}
		result = resp
		return nil
	}); err != nil {
		return agent.A2ATaskResult{}, err
	}

	switch v := result.(type) {
	case *a2asdk.Task:
		return parseTaskResultFromSDK(req.AgentID, "", sdkMethodSendMessage, v), nil
	case *a2asdk.Message:
		return parseMessageResultFromSDK(req.AgentID, sdkMethodSendMessage, v), nil
	default:
		return agent.A2ATaskResult{}, fmt.Errorf("unsupported a2a sdk send result type: %T", result)
	}
}

func (p *Provider) GetTask(ctx context.Context, req agent.A2ATaskQuery) (agent.A2ATaskResult, error) {
	target, err := p.loadEnabledAgent(req.AgentID)
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	client, err := p.buildSDKClient(ctx, target, true)
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	params := &a2asdk.TaskQueryParams{ID: a2asdk.TaskID(strings.TrimSpace(req.TaskID))}
	var task *a2asdk.Task
	if err := p.callWithRetry(ctx, sdkMethodGetTask, func() error {
		resp, callErr := client.GetTask(ctx, params)
		if callErr != nil {
			return callErr
		}
		task = resp
		return nil
	}); err != nil {
		return agent.A2ATaskResult{}, err
	}
	if task == nil {
		return agent.A2ATaskResult{}, fmt.Errorf("a2a sdk get task returned nil")
	}
	return parseTaskResultFromSDK(req.AgentID, req.TaskID, sdkMethodGetTask, task), nil
}

func (p *Provider) CancelTask(ctx context.Context, req agent.A2ATaskQuery) (agent.A2ATaskResult, error) {
	target, err := p.loadEnabledAgent(req.AgentID)
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	client, err := p.buildSDKClient(ctx, target, true)
	if err != nil {
		return agent.A2ATaskResult{}, err
	}
	params := &a2asdk.TaskIDParams{ID: a2asdk.TaskID(strings.TrimSpace(req.TaskID))}
	var task *a2asdk.Task
	if err := p.callWithRetry(ctx, sdkMethodCancelTask, func() error {
		resp, callErr := client.CancelTask(ctx, params)
		if callErr != nil {
			return callErr
		}
		task = resp
		return nil
	}); err != nil {
		return agent.A2ATaskResult{}, err
	}
	if task == nil {
		return agent.A2ATaskResult{}, fmt.Errorf("a2a sdk cancel task returned nil")
	}
	return parseTaskResultFromSDK(req.AgentID, req.TaskID, sdkMethodCancelTask, task), nil
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

func buildSendMessageParams(req agent.A2ASendRequest) *a2asdk.MessageSendParams {
	message := a2asdk.NewMessage(a2asdk.MessageRoleUser, a2asdk.TextPart{Text: strings.TrimSpace(req.Message)})
	metadata := cloneMetadata(req.Metadata)
	if sessionID := strings.TrimSpace(req.SessionID); sessionID != "" {
		if metadata == nil {
			metadata = make(map[string]any, 1)
		}
		metadata["session_id"] = sessionID
	}
	blocking := false
	return &a2asdk.MessageSendParams{
		Message: message,
		Config: &a2asdk.MessageSendConfig{
			Blocking: &blocking,
		},
		Metadata: metadata,
	}
}

func cloneMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (p *Provider) buildSDKClient(ctx context.Context, target mcp.A2AAgent, preferEndpoint bool) (*a2aclient.Client, error) {
	options := []a2aclient.FactoryOption{
		a2aclient.WithJSONRPCTransport(p.http),
		a2aclient.WithConfig(a2aclient.Config{Polling: true}),
	}
	if token := strings.TrimSpace(target.AuthToken); token != "" {
		options = append(options, a2aclient.WithInterceptors(
			a2aclient.NewStaticCallMetaInjector(a2aclient.CallMeta{
				"Authorization": {"Bearer " + token},
			}),
		))
	}

	endpoint := strings.TrimSpace(target.Endpoint)
	cardURL := strings.TrimSpace(target.AgentCardURL)
	if preferEndpoint && endpoint != "" {
		return newSDKClientFromEndpoint(ctx, endpoint, options)
	}
	if cardURL != "" {
		card, err := p.resolveAgentCard(ctx, target)
		if err != nil {
			return nil, err
		}
		client, err := a2aclient.NewFromCard(ctx, card, options...)
		if err != nil {
			return nil, fmt.Errorf("create a2a sdk client from card: %w", err)
		}
		return client, nil
	}
	if endpoint == "" {
		return nil, fmt.Errorf("a2a endpoint is required")
	}
	return newSDKClientFromEndpoint(ctx, endpoint, options)
}

func newSDKClientFromEndpoint(
	ctx context.Context,
	endpoint string,
	options []a2aclient.FactoryOption,
) (*a2aclient.Client, error) {
	endpoints := []a2asdk.AgentInterface{
		{Transport: a2asdk.TransportProtocolJSONRPC, URL: endpoint},
	}
	client, err := a2aclient.NewFromEndpoints(ctx, endpoints, options...)
	if err != nil {
		return nil, fmt.Errorf("create a2a sdk client from endpoint: %w", err)
	}
	return client, nil
}

func (p *Provider) resolveAgentCard(ctx context.Context, target mcp.A2AAgent) (*a2asdk.AgentCard, error) {
	baseURL, opts, err := buildResolverInput(target)
	if err != nil {
		return nil, err
	}
	resolver := p.cardResolver
	if resolver == nil {
		resolver = agentcard.NewResolver(p.http)
	}
	card, err := resolver.Resolve(ctx, baseURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("resolve agent card: %w", err)
	}
	if len(card.Skills) == 0 {
		return nil, fmt.Errorf("agent card skills is required")
	}
	return card, nil
}

func buildResolverInput(target mcp.A2AAgent) (string, []agentcard.ResolveOption, error) {
	rawCardURL := strings.TrimSpace(target.AgentCardURL)
	if rawCardURL == "" {
		return "", nil, fmt.Errorf("agent card url is required")
	}
	parsed, err := url.Parse(rawCardURL)
	if err != nil {
		return "", nil, fmt.Errorf("parse agent card url: %w", err)
	}
	if strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return "", nil, fmt.Errorf("agent card url must include scheme and host")
	}

	baseURL := parsed.Scheme + "://" + parsed.Host
	opts := make([]agentcard.ResolveOption, 0, 2)
	pathWithQuery := strings.TrimSpace(parsed.EscapedPath())
	if query := strings.TrimSpace(parsed.RawQuery); query != "" {
		pathWithQuery = pathWithQuery + "?" + query
	}
	if pathWithQuery != "" && pathWithQuery != "/" && pathWithQuery != "/.well-known/agent-card.json" {
		opts = append(opts, agentcard.WithPath(pathWithQuery))
	}
	if token := strings.TrimSpace(target.AuthToken); token != "" {
		opts = append(opts, agentcard.WithRequestHeader("Authorization", "Bearer "+token))
	}
	return baseURL, opts, nil
}

func (p *Provider) callWithRetry(ctx context.Context, method string, fn func() error) error {
	maxAttempts := methodAttemptLimit(method)
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if !shouldRetryRPCError(lastErr) || attempt >= maxAttempts {
			break
		}
		if err := waitRetryBackoff(ctx, attempt); err != nil {
			return err
		}
	}
	if maxAttempts > 1 {
		return fmt.Errorf("%w (attempts=%d)", lastErr, maxAttempts)
	}
	return lastErr
}

func methodAttemptLimit(method string) int {
	if strings.EqualFold(strings.TrimSpace(method), sdkMethodSendMessage) {
		return 1
	}
	return maxStatusMethodAttempts
}

func shouldRetryRPCError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
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
