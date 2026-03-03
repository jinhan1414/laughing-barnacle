package web

import (
	"fmt"
	"laughing-barnacle/internal/conversation"
	"math"
	"strconv"
	"strings"
)

const (
	tokenScaleK = 1000
	tokenScaleM = 1000000
)

func formatChatUsageLabel(usage *conversation.TokenUsage) string {
	if usage == nil {
		return "Tokens: -"
	}
	prompt := maxTokenValue(usage.PromptTokens)
	completion := maxTokenValue(usage.CompletionTokens)
	cached := maxTokenValue(usage.CachedTokens)
	total := maxTokenValue(usage.TotalTokens)
	if total == 0 {
		total = prompt + completion
	}
	if total == 0 && prompt == 0 && completion == 0 && cached == 0 {
		return "Tokens: -"
	}
	return fmt.Sprintf(
		"Tokens: 总 %s | 输 %s | 出 %s | 缓 %s",
		formatTokenCompact(total),
		formatTokenCompact(prompt),
		formatTokenCompact(completion),
		formatTokenCompact(cached),
	)
}

func formatTokenCompact(value int) string {
	if value <= 0 {
		return "0"
	}
	if value >= tokenScaleM {
		return compactTokenWithSuffix(float64(value)/tokenScaleM, "M")
	}
	if value >= tokenScaleK {
		return compactTokenWithSuffix(float64(value)/tokenScaleK, "K")
	}
	return strconv.Itoa(value)
}

func compactTokenWithSuffix(raw float64, suffix string) string {
	rounded := math.Round(raw*10) / 10
	whole := math.Round(rounded)
	if math.Abs(rounded-whole) < 0.001 {
		return strconv.FormatInt(int64(whole), 10) + suffix
	}
	text := strconv.FormatFloat(rounded, 'f', 1, 64)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	return text + suffix
}

func maxTokenValue(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
