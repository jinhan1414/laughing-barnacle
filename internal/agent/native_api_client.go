package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const nativeAPITimeout = 20 * time.Second

type nativeAPIResult struct {
	StatusCode int    `json:"status_code"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Data       any    `json:"data,omitempty"`
}

func (a *Agent) doLocalAPIRequest(
	ctx context.Context,
	method string,
	path string,
	query map[string]string,
	payload map[string]any,
) (string, error) {
	reqURL, err := buildLocalAPIURL(a.localAPIBaseURL(), path, query)
	if err != nil {
		return "", err
	}

	var body io.Reader
	if len(payload) > 0 {
		data, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return "", fmt.Errorf("invalid payload: %w", marshalErr)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return "", fmt.Errorf("build local api request: %w", err)
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: nativeAPITimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("local api request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read local api response: %w", err)
	}

	var data any
	if len(strings.TrimSpace(string(raw))) > 0 {
		if unmarshalErr := json.Unmarshal(raw, &data); unmarshalErr != nil {
			return "", fmt.Errorf("local api returned invalid json: %w", unmarshalErr)
		}
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("local api %s %s failed: status=%d body=%s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	return marshalNativeAPIResult(nativeAPIResult{
		StatusCode: resp.StatusCode,
		Method:     strings.ToUpper(strings.TrimSpace(method)),
		Path:       path,
		Data:       data,
	})
}

func buildLocalAPIURL(baseURL string, path string, query map[string]string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || strings.TrimSpace(base.Scheme) == "" || strings.TrimSpace(base.Host) == "" {
		return "", fmt.Errorf("invalid local api base url")
	}
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/api/") {
		return "", fmt.Errorf("path must start with /api/")
	}

	target := *base
	target.Path = strings.TrimRight(base.Path, "/") + path
	values := target.Query()
	for key, value := range query {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		values.Set(key, strings.TrimSpace(value))
	}
	target.RawQuery = values.Encode()
	return target.String(), nil
}

func marshalNativeAPIResult(result nativeAPIResult) (string, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal tool output: %w", err)
	}
	return string(data), nil
}
