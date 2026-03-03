package cerber

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"laughing-barnacle/internal/llm"
	"laughing-barnacle/internal/llmlog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (c *Client) appendLog(
	req llm.ChatRequest,
	usage llm.TokenUsage,
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
		Purpose:          req.Purpose,
		Provider:         "cerber",
		Model:            "cerber/" + req.Model,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		CachedTokens:     usage.CachedTokens,
		DurationMS:       duration.Milliseconds(),
		StatusCode:       statusCode,
		Attempts:         attempts,
		Request:          prettyJSONForLog(requestBody),
		Response:         prettyJSONForLog(responseBody),
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
		Usage:       normalizeTokenUsage(parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, parsed.Usage.TotalTokens),
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
