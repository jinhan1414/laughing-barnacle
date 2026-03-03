package web

import (
	"laughing-barnacle/internal/llmlog"
	"testing"
)

func TestBuildTokenStatsPage_ByProvider(t *testing.T) {
	entries := []llmlog.Entry{
		{
			Provider:         "openai-codex",
			Model:            "openai-codex/gpt-5-codex",
			PromptTokens:     1200,
			CompletionTokens: 300,
			TotalTokens:      1500,
			CachedTokens:     150,
		},
		{
			Provider:         "cerber",
			Model:            "cerber/gpt-4o-mini",
			PromptTokens:     500,
			CompletionTokens: 500,
			TotalTokens:      1000,
			CachedTokens:     200,
		},
	}

	view := buildTokenStatsPage(entries)
	if !view.HasData {
		t.Fatalf("expected HasData=true")
	}
	if view.Total.TotalTokens != "2.5K" {
		t.Fatalf("unexpected total tokens: %+v", view.Total)
	}
	if view.Total.PromptTokens != "1.7K" {
		t.Fatalf("unexpected total prompt tokens: %+v", view.Total)
	}
	if len(view.ByChannel) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(view.ByChannel))
	}
	if view.ByChannel[0].Channel != "openai-codex" || view.ByChannel[0].TotalTokens != "1.5K" {
		t.Fatalf("unexpected first channel: %+v", view.ByChannel[0])
	}
}

func TestBuildTokenStatsPage_FallbackParseResponseUsage(t *testing.T) {
	entries := []llmlog.Entry{
		{
			Model: "openai-codex/gpt-5-codex",
			Response: `{
  "output_text": "ok",
  "usage": {
    "input_tokens": 2100,
    "output_tokens": 400,
    "total_tokens": 2500,
    "input_tokens_details": {"cached_tokens": 900}
  }
}`,
		},
	}

	view := buildTokenStatsPage(entries)
	if !view.HasData {
		t.Fatalf("expected HasData=true for parsed usage fallback")
	}
	if view.Total.TotalTokens != "2.5K" || view.Total.CachedTokens != "900" {
		t.Fatalf("unexpected total usage: %+v", view.Total)
	}
	if len(view.ByChannel) != 1 || view.ByChannel[0].Channel != "openai-codex" {
		t.Fatalf("unexpected channels: %+v", view.ByChannel)
	}
}
