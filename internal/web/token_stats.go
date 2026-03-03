package web

import (
	"encoding/json"
	"laughing-barnacle/internal/llmlog"
	"sort"
	"strings"
)

type tokenStatsPageView struct {
	HasData   bool
	Total     tokenUsageView
	ByChannel []tokenChannelUsageView
}

type tokenUsageView struct {
	RequestCount     int
	PromptTokens     string
	CompletionTokens string
	TotalTokens      string
	CachedTokens     string
}

type tokenChannelUsageView struct {
	Channel string
	tokenUsageView
}

type tokenUsageCounter struct {
	requestCount     int
	promptTokens     int
	completionTokens int
	totalTokens      int
	cachedTokens     int
}

func buildTokenStatsPage(entries []llmlog.Entry) tokenStatsPageView {
	byChannel := make(map[string]tokenUsageCounter)
	total := tokenUsageCounter{}

	for _, entry := range entries {
		channel := resolveTokenChannel(entry)
		usage := resolveTokenUsage(entry)
		usage.requestCount = 1
		total = mergeTokenUsage(total, usage)
		byChannel[channel] = mergeTokenUsage(byChannel[channel], usage)
	}

	if total.totalTokens == 0 && total.promptTokens == 0 && total.completionTokens == 0 && total.cachedTokens == 0 {
		return tokenStatsPageView{}
	}

	keys := make([]string, 0, len(byChannel))
	for key := range byChannel {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := byChannel[keys[i]]
		right := byChannel[keys[j]]
		if left.totalTokens != right.totalTokens {
			return left.totalTokens > right.totalTokens
		}
		return keys[i] < keys[j]
	})

	channels := make([]tokenChannelUsageView, 0, len(keys))
	for _, key := range keys {
		channels = append(channels, tokenChannelUsageView{
			Channel:        key,
			tokenUsageView: toTokenUsageView(byChannel[key]),
		})
	}
	return tokenStatsPageView{
		HasData:   true,
		Total:     toTokenUsageView(total),
		ByChannel: channels,
	}
}

func toTokenUsageView(counter tokenUsageCounter) tokenUsageView {
	return tokenUsageView{
		RequestCount:     counter.requestCount,
		PromptTokens:     formatTokenCompact(counter.promptTokens),
		CompletionTokens: formatTokenCompact(counter.completionTokens),
		TotalTokens:      formatTokenCompact(counter.totalTokens),
		CachedTokens:     formatTokenCompact(counter.cachedTokens),
	}
}

func mergeTokenUsage(left, right tokenUsageCounter) tokenUsageCounter {
	return tokenUsageCounter{
		requestCount:     left.requestCount + right.requestCount,
		promptTokens:     left.promptTokens + right.promptTokens,
		completionTokens: left.completionTokens + right.completionTokens,
		totalTokens:      left.totalTokens + right.totalTokens,
		cachedTokens:     left.cachedTokens + right.cachedTokens,
	}
}

func resolveTokenChannel(entry llmlog.Entry) string {
	if provider := strings.TrimSpace(entry.Provider); provider != "" {
		return provider
	}
	model := strings.TrimSpace(entry.Model)
	if model == "" {
		return "unknown"
	}
	if slash := strings.Index(model, "/"); slash > 0 {
		return strings.TrimSpace(model[:slash])
	}
	return "cerber"
}

func resolveTokenUsage(entry llmlog.Entry) tokenUsageCounter {
	counter := tokenUsageCounter{
		promptTokens:     maxTokenValue(entry.PromptTokens),
		completionTokens: maxTokenValue(entry.CompletionTokens),
		totalTokens:      maxTokenValue(entry.TotalTokens),
		cachedTokens:     maxTokenValue(entry.CachedTokens),
	}
	if counter.totalTokens == 0 {
		counter.totalTokens = counter.promptTokens + counter.completionTokens
	}
	if counter.totalTokens > 0 || counter.promptTokens > 0 || counter.completionTokens > 0 || counter.cachedTokens > 0 {
		return counter
	}
	fallback := parseUsageFromResponse(entry.Response)
	if fallback.totalTokens == 0 {
		fallback.totalTokens = fallback.promptTokens + fallback.completionTokens
	}
	return fallback
}

func parseUsageFromResponse(raw string) tokenUsageCounter {
	text := strings.TrimSpace(raw)
	if text == "" {
		return tokenUsageCounter{}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return tokenUsageCounter{}
	}
	usageAny, ok := payload["usage"]
	if !ok {
		return tokenUsageCounter{}
	}
	usage, ok := usageAny.(map[string]any)
	if !ok {
		return tokenUsageCounter{}
	}

	prompt := intFromAny(usage["prompt_tokens"])
	if prompt == 0 {
		prompt = intFromAny(usage["input_tokens"])
	}
	completion := intFromAny(usage["completion_tokens"])
	if completion == 0 {
		completion = intFromAny(usage["output_tokens"])
	}
	cached := intFromAny(usage["cached_tokens"])
	if cached == 0 {
		cached = extractCachedTokensFromDetails(usage)
	}
	total := intFromAny(usage["total_tokens"])
	return tokenUsageCounter{
		promptTokens:     maxTokenValue(prompt),
		completionTokens: maxTokenValue(completion),
		totalTokens:      maxTokenValue(total),
		cachedTokens:     maxTokenValue(cached),
	}
}

func extractCachedTokensFromDetails(usage map[string]any) int {
	if details, ok := usage["input_tokens_details"].(map[string]any); ok {
		if cached := intFromAny(details["cached_tokens"]); cached > 0 {
			return cached
		}
	}
	if details, ok := usage["prompt_tokens_details"].(map[string]any); ok {
		if cached := intFromAny(details["cached_tokens"]); cached > 0 {
			return cached
		}
	}
	return 0
}

func intFromAny(raw any) int {
	switch value := raw.(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	case json.Number:
		out, _ := value.Int64()
		return int(out)
	default:
		return 0
	}
}
