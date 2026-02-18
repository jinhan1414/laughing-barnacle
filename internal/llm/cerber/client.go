package cerber

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"laughing-barnacle/internal/llm"
	"laughing-barnacle/internal/llmlog"
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
			c.appendLog(req, payloadBytes, respBody, statusCode, time.Since(start), nil, attempts)
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

	c.appendLog(req, payloadBytes, lastRespBody, lastStatus, time.Since(start), lastErr, attempts)
	return llm.ChatResponse{}, lastErr
}

func (c *Client) appendLog(
	req llm.ChatRequest,
	requestBody []byte,
	responseBody []byte,
	statusCode int,
	duration time.Duration,
	err error,
	attempts int,
) {
	if c.logs == nil {
		return
	}
	if attempts <= 0 {
		attempts = 1
	}

	entry := llmlog.Entry{
		Purpose:    req.Purpose,
		Model:      req.Model,
		DurationMS: duration.Milliseconds(),
		StatusCode: statusCode,
		Attempts:   attempts,
		Request:    prettyJSONForLog(requestBody),
		Response:   prettyJSONForLog(responseBody),
	}
	if err != nil {
		entry.Error = err.Error()
	}
	c.logs.Add(entry)
}

func (c *Client) chatOnce(ctx context.Context, payloadBytes []byte) (llm.ChatResponse, []byte, int, time.Duration, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/chat/completions",
		bytes.NewReader(payloadBytes),
	)
	if err != nil {
		return llm.ChatResponse{}, nil, 0, 0, fmt.Errorf("build request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return llm.ChatResponse{}, nil, 0, 0, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return llm.ChatResponse{}, nil, httpResp.StatusCode, 0, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode >= http.StatusBadRequest {
		retryAfter := parseRetryAfterHeader(httpResp.Header.Get("Retry-After"))
		err = fmt.Errorf("cerber status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
		return llm.ChatResponse{}, respBody, httpResp.StatusCode, retryAfter, err
	}

	var parsed chatResponsePayload
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return llm.ChatResponse{}, respBody, httpResp.StatusCode, 0, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return llm.ChatResponse{}, respBody, httpResp.StatusCode, 0, fmt.Errorf("empty choices in response")
	}

	content := extractContent(parsed.Choices[0].Message.Content)
	toolCalls := parsed.Choices[0].Message.ToolCalls
	if strings.TrimSpace(content) == "" && len(toolCalls) == 0 {
		return llm.ChatResponse{}, respBody, httpResp.StatusCode, 0, fmt.Errorf("empty content and tool_calls in response")
	}

	return llm.ChatResponse{
		Content:     content,
		ToolCalls:   toolCalls,
		RawResponse: string(respBody),
	}, respBody, httpResp.StatusCode, 0, nil
}

func (c *Client) shouldRetry(attempt, maxAttempts, statusCode int, retryAfter time.Duration, err error) bool {
	if attempt >= maxAttempts {
		return false
	}
	if retryAfter > 0 {
		return true
	}
	if statusCode == http.StatusTooManyRequests || statusCode == http.StatusRequestTimeout {
		return true
	}
	if statusCode >= http.StatusInternalServerError {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func (c *Client) retryDelay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > c.retryMaxDelay {
			return c.retryMaxDelay
		}
		return retryAfter
	}
	delay := c.retryBaseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= c.retryMaxDelay {
			return c.retryMaxDelay
		}
	}
	return delay
}

func parseRetryAfterHeader(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(raw)
	if err != nil {
		return 0
	}
	wait := time.Until(when)
	if wait <= 0 {
		return 0
	}
	return wait
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func prettyJSONForLog(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	var out bytes.Buffer
	if err := json.Indent(&out, trimmed, "", "  "); err == nil {
		return out.String()
	}
	return string(trimmed)
}

func extractContent(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := extractTextFromPart(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func extractTextFromPart(item any) string {
	m, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	text, ok := m["text"].(string)
	if !ok {
		return ""
	}
	return text
}
