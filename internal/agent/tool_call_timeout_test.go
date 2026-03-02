package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"testing"
	"time"
)

type timeoutProbeTools struct {
	listed   []llm.ToolDefinition
	deadline time.Time
	hasDead  bool
}

func (p *timeoutProbeTools) ListTools(context.Context) ([]llm.ToolDefinition, error) {
	return p.listed, nil
}

func (p *timeoutProbeTools) CallTool(ctx context.Context, _ llm.ToolCall) (string, error) {
	p.deadline, p.hasDead = ctx.Deadline()
	return `{"ok":true}`, nil
}

func TestHandleUserMessage_ToolCallGetsIndependentTwoMinuteBudget(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "done"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "call_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      "weather__query",
							Arguments: `{"city":"beijing"}`,
						},
					},
				},
				nil,
			},
		},
	}
	probe := &timeoutProbeTools{
		listed: []llm.ToolDefinition{
			{
				Type: "function",
				Function: llm.ToolFunctionDefinition{
					Name: "weather__query",
				},
			},
		},
	}

	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          2,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, probe)

	start := time.Now()
	if _, err := agentSvc.HandleUserMessage(context.Background(), "今天北京天气"); err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if !probe.hasDead {
		t.Fatalf("expected tool call deadline")
	}

	budget := probe.deadline.Sub(start)
	if budget < 100*time.Second || budget > 2*time.Minute+5*time.Second {
		t.Fatalf("expected per-tool budget near 2 minutes, got %s", budget)
	}
}
