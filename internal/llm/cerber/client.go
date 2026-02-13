package cerber

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"laughing-barnacle/internal/llm"
	"laughing-barnacle/internal/llmlog"
)

type Config struct {
	BaseURL    string
	APIKey     string
	Timeout    time.Duration
	HTTPClient *http.Client
	LogStore   *llmlog.Store
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
	logs    *llmlog.Store
}

func NewClient(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}

	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		http:    httpClient,
		logs:    cfg.LogStore,
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

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/chat/completions",
		bytes.NewReader(payloadBytes),
	)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("build request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		c.appendLog(req, payloadBytes, nil, 0, time.Since(start), err)
		return llm.ChatResponse{}, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		c.appendLog(req, payloadBytes, nil, httpResp.StatusCode, time.Since(start), err)
		return llm.ChatResponse{}, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode >= http.StatusBadRequest {
		err = fmt.Errorf("cerber status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
		c.appendLog(req, payloadBytes, respBody, httpResp.StatusCode, time.Since(start), err)
		return llm.ChatResponse{}, err
	}

	var parsed chatResponsePayload
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		c.appendLog(req, payloadBytes, respBody, httpResp.StatusCode, time.Since(start), err)
		return llm.ChatResponse{}, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		err = fmt.Errorf("empty choices in response")
		c.appendLog(req, payloadBytes, respBody, httpResp.StatusCode, time.Since(start), err)
		return llm.ChatResponse{}, err
	}

	content := extractContent(parsed.Choices[0].Message.Content)
	toolCalls := parsed.Choices[0].Message.ToolCalls
	if strings.TrimSpace(content) == "" && len(toolCalls) == 0 {
		err = fmt.Errorf("empty content and tool_calls in response")
		c.appendLog(req, payloadBytes, respBody, httpResp.StatusCode, time.Since(start), err)
		return llm.ChatResponse{}, err
	}

	c.appendLog(req, payloadBytes, respBody, httpResp.StatusCode, time.Since(start), nil)

	return llm.ChatResponse{
		Content:     content,
		ToolCalls:   toolCalls,
		RawResponse: string(respBody),
	}, nil
}

func (c *Client) appendLog(
	req llm.ChatRequest,
	requestBody []byte,
	responseBody []byte,
	statusCode int,
	duration time.Duration,
	err error,
) {
	if c.logs == nil {
		return
	}

	entry := llmlog.Entry{
		Purpose:    req.Purpose,
		Model:      req.Model,
		DurationMS: duration.Milliseconds(),
		StatusCode: statusCode,
		Request:    prettyJSONForLog(requestBody),
		Response:   prettyJSONForLog(responseBody),
	}
	if err != nil {
		entry.Error = err.Error()
	}
	c.logs.Add(entry)
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
