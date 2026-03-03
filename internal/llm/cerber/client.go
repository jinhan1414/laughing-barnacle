package cerber

import (
	"context"
	"encoding/json"
	"fmt"
	"laughing-barnacle/internal/llm"
	"laughing-barnacle/internal/llmlog"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	BaseURL        string
	APIKey         string
	Timeout        time.Duration
	HTTPClient     *http.Client
	LogStore       *llmlog.Store
	MaxRetries     int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
}

type Client struct {
	baseURL        string
	apiKey         string
	http           *http.Client
	logs           *llmlog.Store
	maxRetries     int
	retryBaseDelay time.Duration
	retryMaxDelay  time.Duration
}

func NewClient(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	retryBaseDelay := cfg.RetryBaseDelay
	if retryBaseDelay <= 0 {
		retryBaseDelay = 700 * time.Millisecond
	}
	retryMaxDelay := cfg.RetryMaxDelay
	if retryMaxDelay <= 0 {
		retryMaxDelay = 8 * time.Second
	}
	if retryMaxDelay < retryBaseDelay {
		retryMaxDelay = retryBaseDelay
	}

	return &Client{
		baseURL:        strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:         cfg.APIKey,
		http:           httpClient,
		logs:           cfg.LogStore,
		maxRetries:     maxRetries,
		retryBaseDelay: retryBaseDelay,
		retryMaxDelay:  retryMaxDelay,
	}
}

type chatRequestPayload struct {
	Model       string               `json:"model"`
	Messages    []llm.Message        `json:"messages"`
	Tools       []llm.ToolDefinition `json:"tools,omitempty"`
	Temperature float64              `json:"temperature,omitempty"`
	Stream      bool                 `json:"stream"`
}

type chatResponsePayload struct {
	Choices []struct {
		Message struct {
			Content   any            `json:"content"`
			ToolCalls []llm.ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *Client) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	if req.Model == "" {
		return llm.ChatResponse{}, fmt.Errorf("model is required")
	}
	if len(req.Messages) == 0 {
		return llm.ChatResponse{}, fmt.Errorf("messages are required")
	}

	payload := chatRequestPayload{
		Model:       req.Model,
		Messages:    req.Messages,
		Tools:       req.Tools,
		Temperature: req.Temperature,
		Stream:      false,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	start := time.Now()
	maxAttempts := c.maxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var (
		lastErr      error
		lastRespBody []byte
		lastStatus   int
		attempts     int
	)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attempts = attempt
		resp, respBody, statusCode, retryAfter, err := c.chatOnce(ctx, payloadBytes)
		lastRespBody = respBody
		lastStatus = statusCode
		if err == nil {
			c.appendLog(req, resp.Usage, payloadBytes, respBody, statusCode, time.Since(start), nil, attempts)
			return resp, nil
		}

		lastErr = err
		if !c.shouldRetry(attempt, maxAttempts, statusCode, retryAfter, err) {
			break
		}
		delay := c.retryDelay(attempt, retryAfter)
		if waitErr := sleepContext(ctx, delay); waitErr != nil {
			lastErr = waitErr
			break
		}
	}

	c.appendLog(req, llm.TokenUsage{}, payloadBytes, lastRespBody, lastStatus, time.Since(start), lastErr, attempts)
	return llm.ChatResponse{}, lastErr
}
