package agent

import (
	"context"
	"strings"
	"testing"

	"laughing-barnacle/internal/conversation"
)

func TestBuildSkillIndexPrompt_IncludesResourceHints(t *testing.T) {
	prompt := buildSkillIndexPrompt([]string{"skill_id=fixture | name=Fixture | description=demo"}, 1)
	required := []string{
		`context__read(resource="skills", action="index", id="<skill_id>")`,
		`context__read(resource="skills", action="read", id="<skill_id>", path="references/<file>.md")`,
		"若 skill 含脚本",
	}
	for _, item := range required {
		if !strings.Contains(prompt, item) {
			t.Fatalf("expected prompt to contain %q, got %q", item, prompt)
		}
	}
}

func TestHandleUserMessage_RuntimePromptMentionsSkillResourceBoundaries(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{responses: map[string][]string{"chat_reply": {"ok"}}}
	agentSvc := New(Config{Model: "test-model", MaxToolCallRounds: 2, SystemPrompt: "system", CompressionSystemPrompt: "compressor"}, store, fakeLLM, nil)

	_, err := agentSvc.HandleUserMessage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	found := ""
	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "skill 文档与 references 优先用 context__read") {
			found = msg.Content
			break
		}
	}
	if found == "" || !strings.Contains(found, "skill 脚本仅在确有必要时使用 bash 执行") {
		t.Fatalf("expected runtime prompt skill resource guidance, got %q", found)
	}
}
