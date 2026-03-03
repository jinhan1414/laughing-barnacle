package openaicodex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"laughing-barnacle/internal/llmgateway"
	"laughing-barnacle/internal/llmlog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *Adapter) chatWithRetry(
	ctx context.Context,
	auth authContext,
	payloadBytes []byte,
	promptCacheSessionID string,
) (llmgateway.CanonicalChatResponse, int, []byte, error, int) {
	maxAttempts := a.maxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var (
		lastErr      error
		lastStatus   int
		lastBody     []byte
		lastAttempts int
	)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastAttempts = attempt
		resp, body, statusCode, retryAfter, err := a.chatOnce(
			ctx,
			auth,
			payloadBytes,
			promptCacheSessionID,
		)
		lastStatus = statusCode
		lastBody = body
		if err == nil {
			return resp, statusCode, body, nil, attempt
		}
		lastErr = err
		if !shouldRetry(attempt, maxAttempts, statusCode, retryAfter, err) {
			break
		}
		if sleepErr := sleepContext(ctx, retryDelay(attempt, retryAfter, a.retryBaseDelay, a.retryMaxDelay)); sleepErr != nil {
			lastErr = sleepErr
			break
		}
	}
	return llmgateway.CanonicalChatResponse{}, lastStatus, lastBody, lastErr, lastAttempts
}

func (a *Adapter) chatOnce(
	ctx context.Context,
	auth authContext,
	payloadBytes []byte,
	promptCacheSessionID string,
) (llmgateway.CanonicalChatResponse, []byte, int, time.Duration, error) {
	endpoint := resolveEndpointURL(a.baseURL, auth)
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(payloadBytes),
	)
	if err != nil {
		return llmgateway.CanonicalChatResponse{}, nil, 0, 0, providerError(0, fmt.Errorf("build request: %w", err))
	}
	httpReq.Header.Set("Authorization", "Bearer "+auth.Token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if strings.TrimSpace(auth.AccountID) != "" {
		httpReq.Header.Set("ChatGPT-Account-Id", auth.AccountID)
	}
	if strings.TrimSpace(promptCacheSessionID) != "" {
		httpReq.Header.Set("session_id", promptCacheSessionID)
	}

	httpResp, err := a.http.Do(httpReq)
	if err != nil {
		return llmgateway.CanonicalChatResponse{}, nil, 0, 0, providerError(0, fmt.Errorf("request failed: %w", err))
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return llmgateway.CanonicalChatResponse{}, nil, httpResp.StatusCode, 0, providerError(httpResp.StatusCode, fmt.Errorf("read response: %w", err))
	}
	if httpResp.StatusCode >= http.StatusBadRequest {
		retryAfter := parseRetryAfterHeader(httpResp.Header.Get("Retry-After"))
		err = providerError(httpResp.StatusCode, fmt.Errorf("codex status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody))))
		return llmgateway.CanonicalChatResponse{}, respBody, httpResp.StatusCode, retryAfter, err
	}

	parsed, err := parseBody(respBody, httpResp.Header.Get("Content-Type"), auth)
	if err != nil {
		return llmgateway.CanonicalChatResponse{}, respBody, httpResp.StatusCode, 0, providerError(httpResp.StatusCode, err)
	}
	return parsed, respBody, httpResp.StatusCode, 0, nil
}

func parseBody(
	respBody []byte,
	contentType string,
	auth authContext,
) (llmgateway.CanonicalChatResponse, error) {
	if shouldParseEventStream(contentType, auth, respBody) {
		return parseEventStreamResponse(respBody)
	}
	return parseResponse(respBody)
}

func shouldParseEventStream(contentType string, auth authContext, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return true
	}
	return isChatGPTAuth(auth) && bytes.Contains(body, []byte("event: "))
}

func parseEventStreamResponse(raw []byte) (llmgateway.CanonicalChatResponse, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)
	chunk := strings.Builder{}
	text := strings.Builder{}
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			resp, done, err := handleEventChunk(chunk.String(), &text)
			if done || err != nil {
				return resp, err
			}
			chunk.Reset()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			chunk.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return llmgateway.CanonicalChatResponse{}, fmt.Errorf("read event stream: %w", err)
	}
	resp, done, err := handleEventChunk(chunk.String(), &text)
	if done || err != nil {
		return resp, err
	}
	if strings.TrimSpace(text.String()) == "" {
		return llmgateway.CanonicalChatResponse{}, fmt.Errorf("empty event stream response")
	}
	return llmgateway.CanonicalChatResponse{Content: strings.TrimSpace(text.String())}, nil
}

func handleEventChunk(
	chunk string,
	text *strings.Builder,
) (llmgateway.CanonicalChatResponse, bool, error) {
	payload := strings.TrimSpace(chunk)
	if payload == "" || payload == "[DONE]" {
		return llmgateway.CanonicalChatResponse{}, false, nil
	}
	eventType := readEventType(payload)
	if eventType == "response.output_text.delta" {
		text.WriteString(readEventTextDelta(payload))
		return llmgateway.CanonicalChatResponse{}, false, nil
	}
	if eventType == "response.failed" {
		return llmgateway.CanonicalChatResponse{}, true, readFailedResponse(payload)
	}
	if eventType != "response.completed" {
		return llmgateway.CanonicalChatResponse{}, false, nil
	}
	responseJSON, err := readCompletedResponse(payload)
	if err != nil {
		return llmgateway.CanonicalChatResponse{}, true, err
	}
	resp, err := parseResponse(responseJSON)
	if err != nil {
		return llmgateway.CanonicalChatResponse{}, true, err
	}
	if strings.TrimSpace(resp.Content) == "" {
		resp.Content = strings.TrimSpace(text.String())
	}
	return resp, true, nil
}

func readEventType(payload string) string {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return ""
	}
	return envelope.Type
}

func readEventTextDelta(payload string) string {
	var event struct {
		Delta string `json:"delta"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return ""
	}
	return event.Delta
}

func readCompletedResponse(payload string) ([]byte, error) {
	var event struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return nil, fmt.Errorf("decode completed event: %w", err)
	}
	if len(event.Response) == 0 {
		return nil, fmt.Errorf("completed event missing response")
	}
	return event.Response, nil
}

func readFailedResponse(payload string) error {
	var event struct {
		Response struct {
			Status string `json:"status"`
			Error  struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("decode failed event: %w", err)
	}
	code := strings.TrimSpace(event.Response.Error.Code)
	message := strings.TrimSpace(event.Response.Error.Message)
	status := strings.TrimSpace(event.Response.Status)
	parts := make([]string, 0, 3)
	if code != "" {
		parts = append(parts, code)
	}
	if message != "" {
		parts = append(parts, message)
	}
	if status != "" {
		parts = append(parts, "status="+status)
	}
	if len(parts) == 0 {
		return fmt.Errorf("response.failed without error details")
	}
	return fmt.Errorf("provider rejected event stream: %s", strings.Join(parts, " | "))
}

func providerError(statusCode int, err error) error {
	code := llmgateway.ErrorCodeProviderRequestFailed
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		code = llmgateway.ErrorCodeAuthConfigInvalid
	}
	return &llmgateway.Error{
		Code:       code,
		Provider:   providerName,
		StatusCode: statusCode,
		Message:    "codex provider request failed",
		Cause:      err,
	}
}

func (a *Adapter) appendLog(
	req llmgateway.CanonicalChatRequest,
	usage llmgateway.CanonicalTokenUsage,
	requestBody []byte,
	responseBody []byte,
	statusCode int,
	duration time.Duration,
	err error,
	attempts int,
) {
	if a.logs == nil {
		return
	}
	if attempts <= 0 {
		attempts = 1
	}
	entry := llmlog.Entry{
		Purpose:          req.Purpose,
		Provider:         req.Provider,
		Model:            req.Provider + "/" + req.Model,
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
	a.logs.Add(entry)
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

func shouldRetry(attempt, maxAttempts, statusCode int, retryAfter time.Duration, err error) bool {
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
	return err != nil && errors.As(err, &netErr)
}

func retryDelay(attempt int, retryAfter, baseDelay, maxDelay time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > maxDelay {
			return maxDelay
		}
		return retryAfter
	}
	delay := baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= maxDelay {
			return maxDelay
		}
	}
	return delay
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
