package openaicodex

import (
	"context"
	"encoding/json"
	"fmt"
	"laughing-barnacle/internal/llmgateway"
	"laughing-barnacle/internal/llmlog"
	"net/http"
	"strings"
	"time"
)

const providerName = "openai-codex"

type Config struct {
	BaseURL         string
	APIToken        string
	AuthFilePath    string
	Timeout         time.Duration
	HTTPClient      *http.Client
	Transport       string
	ReasoningEffort string
	LogStore        *llmlog.Store
	MaxRetries      int
	RetryBaseDelay  time.Duration
	RetryMaxDelay   time.Duration
}

type Adapter struct {
	baseURL         string
	apiToken        string
	authFilePath    string
	http            *http.Client
	transport       string
	reasoningEffort string
	logs            *llmlog.Store
	maxRetries      int
	retryBaseDelay  time.Duration
	retryMaxDelay   time.Duration
}

func New(cfg Config) *Adapter {
	baseDelay := normalizeRetryBaseDelay(cfg.RetryBaseDelay)
	maxDelay := normalizeRetryMaxDelay(baseDelay, cfg.RetryMaxDelay)
	return &Adapter{
		baseURL:         strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiToken:        strings.TrimSpace(cfg.APIToken),
		authFilePath:    strings.TrimSpace(cfg.AuthFilePath),
		http:            newHTTPClient(cfg.HTTPClient, cfg.Timeout),
		transport:       strings.TrimSpace(cfg.Transport),
		reasoningEffort: strings.TrimSpace(cfg.ReasoningEffort),
		logs:            cfg.LogStore,
		maxRetries:      normalizeRetries(cfg.MaxRetries),
		retryBaseDelay:  baseDelay,
		retryMaxDelay:   maxDelay,
	}
}

func newHTTPClient(client *http.Client, timeout time.Duration) *http.Client {
	if client != nil {
		return client
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func normalizeRetries(maxRetries int) int {
	if maxRetries < 0 {
		return 0
	}
	return maxRetries
}

func normalizeRetryBaseDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 700 * time.Millisecond
	}
	return delay
}

func normalizeRetryMaxDelay(baseDelay, maxDelay time.Duration) time.Duration {
	if maxDelay <= 0 {
		maxDelay = 8 * time.Second
	}
	if maxDelay < baseDelay && baseDelay > 0 {
		return baseDelay
	}
	return maxDelay
}

func (a *Adapter) Name() string {
	return providerName
}

func (a *Adapter) Chat(ctx context.Context, req llmgateway.CanonicalChatRequest) (llmgateway.CanonicalChatResponse, error) {
	if strings.TrimSpace(req.Model) == "" {
		return llmgateway.CanonicalChatResponse{}, invalidRequestError("model is required")
	}
	if len(req.Messages) == 0 {
		return llmgateway.CanonicalChatResponse{}, invalidRequestError("messages are required")
	}

	auth, err := a.resolveAuthContext()
	if err != nil {
		return llmgateway.CanonicalChatResponse{}, err
	}
	payload := buildPayload(req, a.transport, a.reasoningEffort, auth)
	promptCacheKey := resolvePromptCacheKey(req, auth)
	if promptCacheKey != "" {
		payload["prompt_cache_key"] = promptCacheKey
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return llmgateway.CanonicalChatResponse{}, invalidRequestError(fmt.Sprintf("marshal request: %v", err))
	}

	start := time.Now()
	parsed, statusCode, respBody, callErr, attempts := a.chatWithRetry(
		ctx,
		auth,
		payloadBytes,
		promptCacheKey,
	)
	a.appendLog(req, payloadBytes, respBody, statusCode, time.Since(start), callErr, attempts)
	if callErr != nil {
		return llmgateway.CanonicalChatResponse{}, callErr
	}
	return parsed, nil
}

func invalidRequestError(message string) error {
	return &llmgateway.Error{
		Code:     llmgateway.ErrorCodeInvalidRequest,
		Provider: providerName,
		Message:  message,
	}
}
