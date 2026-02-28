package agent

import (
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

func toConversationUsage(usage llm.TokenUsage) *conversation.TokenUsage {
	normalized, hasUsage := mergeTokenUsage(conversation.TokenUsage{}, false, usage)
	if !hasUsage {
		return nil
	}
	return &normalized
}

func mergeTokenUsage(total conversation.TokenUsage, hasUsage bool, usage llm.TokenUsage) (conversation.TokenUsage, bool) {
	prompt := usage.PromptTokens
	completion := usage.CompletionTokens
	all := usage.TotalTokens
	cached := usage.CachedTokens
	if prompt < 0 {
		prompt = 0
	}
	if completion < 0 {
		completion = 0
	}
	if all < 0 {
		all = 0
	}
	if cached < 0 {
		cached = 0
	}
	if all == 0 {
		all = prompt + completion
	}
	if prompt == 0 && completion == 0 && all == 0 && cached == 0 {
		return total, hasUsage
	}
	total.PromptTokens += prompt
	total.CompletionTokens += completion
	total.TotalTokens += all
	total.CachedTokens += cached
	return total, true
}

func usageOrNil(total conversation.TokenUsage, hasUsage bool) *conversation.TokenUsage {
	if !hasUsage {
		return nil
	}
	out := total
	return &out
}
