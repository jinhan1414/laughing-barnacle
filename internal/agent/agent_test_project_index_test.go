package agent

import (
	"context"
	"strings"
	"testing"

	"laughing-barnacle/internal/conversation"
)

func TestHandleUserMessage_InjectsProjectsIndex(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{
		"chat_reply": {"ok"},
	}}
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
	}, store, fakeLLM, nil)
	agentSvc.SetMemoryProvider(&mockMemory{
		projectIndexLines: []string{
			"project_id=laughing-barnacle | working_dir=E:\\projects\\ai\\laughing-barnacle | relative_path=laughing-barnacle | aliases=barnacle | summary=repo",
		},
	})
	agentSvc.SetProjectRootProvider(&mockProjectRoot{rootDir: `E:\projects\ai`})

	if _, err := agentSvc.HandleUserMessage(context.Background(), "继续开发 barnacle"); err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	content := ""
	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "Projects Index (共") {
			content = msg.Content
			break
		}
	}
	if !strings.Contains(content, "project_id=laughing-barnacle") {
		t.Fatalf("expected project index line, got %q", content)
	}
	if !strings.Contains(content, "metadata.project_id") {
		t.Fatalf("expected codex-local metadata hint, got %q", content)
	}
}
