package web

import (
	"laughing-barnacle/internal/conversation"
	"testing"
)

func TestFormatTokenCompact(t *testing.T) {
	cases := []struct {
		name  string
		input int
		want  string
	}{
		{name: "zero", input: 0, want: "0"},
		{name: "normal", input: 999, want: "999"},
		{name: "k-int", input: 1000, want: "1K"},
		{name: "k-float", input: 1540, want: "1.5K"},
		{name: "m-int", input: 2000000, want: "2M"},
		{name: "m-float", input: 1250000, want: "1.3M"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatTokenCompact(tc.input); got != tc.want {
				t.Fatalf("formatTokenCompact(%d)=%q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatChatUsageLabel(t *testing.T) {
	usage := &conversation.TokenUsage{
		PromptTokens:     1500,
		CompletionTokens: 1200,
		TotalTokens:      2700,
		CachedTokens:     950,
	}
	got := formatChatUsageLabel(usage)
	want := "Tokens: 总 2.7K | 输 1.5K | 出 1.2K | 缓 950"
	if got != want {
		t.Fatalf("formatChatUsageLabel()=%q, want %q", got, want)
	}
}
