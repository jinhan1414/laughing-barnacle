package openaicodex

import (
	"bytes"
	"context"
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
	token string,
	payloadBytes []byte,
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
		resp, body, statusCode, retryAfter, err := a.chatOnce(ctx, token, payloadBytes)
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
	token string,
	payloadBytes []byte,
) (llmgateway.CanonicalChatResponse, []byte, int, time.Duration, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		a.baseURL+"/v1/responses",
		bytes.NewReader(payloadBytes),
	)
	if err != nil {
		return llmgateway.CanonicalChatResponse{}, nil, 0, 0, providerError(0, fmt.Errorf("build request: %w", err))
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

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

	parsed, err := parseResponse(respBody)
	if err != nil {
		return llmgateway.CanonicalChatResponse{}, respBody, httpResp.StatusCode, 0, providerError(httpResp.StatusCode, err)
	}
	return parsed, respBody, httpResp.StatusCode, 0, nil
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
		Purpose:    req.Purpose,
		Model:      req.Provider + "/" + req.Model,
		DurationMS: duration.Milliseconds(),
		StatusCode: statusCode,
		Attempts:   attempts,
		Request:    prettyJSONForLog(requestBody),
		Response:   prettyJSONForLog(responseBody),
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
