package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"strings"
	"testing"
)

func TestHandleUserMessage_IncludesSkillIndexForProgressiveDisclosure(t *testing.T) {
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
		EnforceHumanRoutine:        true,
	}, store, fakeLLM, nil)
	agentSvc.SetSkillProvider(&mockSkills{
		indexLines: []string{
			"skill_id=research-flow | name=检索闭环 | description=先检索再回答",
		},
		promptByID: map[string]string{
			"research-flow": "这是完整技能正文，不应在首轮直接注入。",
		},
	})

	reply, err := agentSvc.HandleUserMessage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}

	msgs := fakeLLM.calls[0].Messages
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}
	content := ""
	for _, msg := range msgs {
		if msg.Role == "system" && strings.Contains(msg.Content, "Skills 索引 (共") {
			content = msg.Content
			break
		}
	}
	if strings.TrimSpace(content) == "" {
		t.Fatalf("skill index not injected")
	}
	if !strings.Contains(content, "context__read(resource=\"skills\", action=\"read\", id=\"<skill_id>\")") {
		t.Fatalf("expected native context read hint, got %q", content)
	}
	if !strings.Contains(content, "无需用户点名") {
		t.Fatalf("expected proactive skill trigger hint, got %q", content)
	}
	if !strings.Contains(content, "单轮默认只读取 1 个最相关技能") {
		t.Fatalf("expected token-saving skill read strategy, got %q", content)
	}
	if strings.Contains(content, "本轮高相关候选") {
		t.Fatalf("unexpected related-skill hint for unrelated query, got %q", content)
	}
	if strings.Contains(content, "完整技能正文") {
		t.Fatalf("should not inject full skill content directly, got %q", content)
	}
	for _, msg := range msgs {
		if msg.Role != "system" {
			continue
		}
		if strings.Contains(msg.Content, "内置工具仅有 bash") {
			t.Fatalf("legacy bash-only policy text should not be injected as system context, got %q", msg.Content)
		}
		if strings.Contains(msg.Content, "/api/memory/read?path=/conversation/archive") || strings.Contains(msg.Content, "内置技能 archive_recall") {
			t.Fatalf("archive recall should not be hardcoded in system prompt, got %q", msg.Content)
		}
	}
}

func TestHandleUserMessage_UsesBuiltinLinuxAndAsyncTaskTools(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"done"},
		},
	}

	agentSvc := New(Config{
		Model:                      "test-model",
		MaxRecentMessages:          10,
		CompressionTriggerMessages: 99,
		CompressionTriggerChars:    99999,
		KeepRecentAfterCompression: 1,
		MaxCompressionLoopsPerTurn: 1,
		MaxToolCallRounds:          3,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)
	reply, err := agentSvc.HandleUserMessage(context.Background(), "test")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "done" {
		t.Fatalf("unexpected reply: %q", reply)
	}

	firstCall := fakeLLM.calls[0]
	if len(firstCall.Tools) != 6 {
		t.Fatalf("expected 6 builtin tools, got %d", len(firstCall.Tools))
	}
	seen := map[string]bool{}
	for _, tool := range firstCall.Tools {
		seen[tool.Function.Name] = true
	}
	required := []string{
		builtinLinuxBashToolName,
		builtinContextReadToolName,
		builtinMaintenanceWriteToolName,
		builtinAsyncTaskSubmitToolName,
		builtinAsyncTaskGetToolName,
		builtinAsyncTaskCancelToolName,
	}
	for _, name := range required {
		if !seen[name] {
			t.Fatalf("missing builtin tool: %s", name)
		}
	}
}

func TestHandleUserMessage_LinuxBashToolCall(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "bash ok"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "call_bash_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      builtinLinuxBashToolName,
							Arguments: `"echo hello-linux-bash"`,
						},
					},
				},
				nil,
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
		MaxToolCallRounds:          3,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)

	reply, err := agentSvc.HandleUserMessage(context.Background(), "run bash")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "bash ok" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}

	secondCall := fakeLLM.calls[1]
	foundToolResult := false
	for _, msg := range secondCall.Messages {
		if msg.Role == "tool" && strings.Contains(msg.Content, "exit_code: 0") && strings.Contains(msg.Content, "hello-linux-bash") {
			foundToolResult = true
			break
		}
	}
	if !foundToolResult {
		t.Fatalf("expected bash tool result in second call")
	}

	_, messages := store.Snapshot()
	if len(messages) != 2 {
		t.Fatalf("expected user + assistant messages, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 1 {
		t.Fatalf("expected one tool call recorded, got %d", len(messages[0].ToolCalls))
	}
	if messages[0].ToolCalls[0].Name != builtinLinuxBashToolName {
		t.Fatalf("unexpected tool call name: %s", messages[0].ToolCalls[0].Name)
	}
}

func TestHandleUserMessage_IncludesToolRuntimeConstraintsPrompt(t *testing.T) {
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
		EnforceHumanRoutine:        true,
	}, store, fakeLLM, nil)

	_, err := agentSvc.HandleUserMessage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}

	msgs := fakeLLM.calls[0].Messages
	found := ""
	for _, msg := range msgs {
		if msg.Role != "system" {
			continue
		}
		if strings.Contains(msg.Content, "仅保留 command 参数") {
			found = msg.Content
			break
		}
	}
	if strings.TrimSpace(found) == "" {
		t.Fatalf("expected tool runtime constraints prompt to be injected")
	}
	if strings.Contains(found, "不要再包 command/cmd 键") {
		t.Fatalf("expected command-only wording removed, got %q", found)
	}
	if !strings.Contains(found, "# Tool & Environment Constraints (工具与环境硬约束)") {
		t.Fatalf("expected markdown heading structure for runtime constraints, got %q", found)
	}
	if !strings.Contains(found, "context__read(resource=\"schedules\", action=\"list\")") {
		t.Fatalf("expected schedules list native-tool constraint, got %q", found)
	}
	if !strings.Contains(found, "context__read") || !strings.Contains(found, "maintenance__write") {
		t.Fatalf("expected native tool routing constraints, got %q", found)
	}
	if !strings.Contains(found, "id,name,description,action=skill:<skill_id>,cron_expr,enabled") {
		t.Fatalf("expected required schedule save fields constraint, got %q", found)
	}
	if !strings.Contains(found, "skills(save/toggle/delete/install)") {
		t.Fatalf("expected maintenance tool whitelist constraint, got %q", found)
	}
	if !strings.Contains(found, "执行一致性硬约束（最高优先级）") {
		t.Fatalf("expected execution consistency hard constraints, got %q", found)
	}
	if !strings.Contains(found, "我还未执行") {
		t.Fatalf("expected explicit not-executed wording for blocked turns, got %q", found)
	}
	if !strings.Contains(found, "tool_name 与 call_id（或 task_id）") {
		t.Fatalf("expected execution evidence fields guidance, got %q", found)
	}
	if !strings.Contains(found, "agent_input 禁止出现“调用 <agent_id>") {
		t.Fatalf("expected no-dispatch guidance for a2a agent_input, got %q", found)
	}
	if preferredShellName() == "cmd" {
		if !strings.Contains(found, "禁止写反斜杠转义引号") {
			t.Fatalf("expected windows quote escape constraint for cmd shell, got %q", found)
		}
		if !strings.Contains(found, "禁止通过 bash 执行任何本地 API 读写") {
			t.Fatalf("expected bash local api prohibition for cmd shell, got %q", found)
		}
	}
	if preferredShellName() == "powershell" || preferredShellName() == "pwsh" {
		if !strings.Contains(found, "Windows PowerShell") {
			t.Fatalf("expected powershell runtime prompt, got %q", found)
		}
		if !strings.Contains(found, "禁止通过 bash 执行任何本地 API 读写") {
			t.Fatalf("expected bash local api prohibition for powershell shell, got %q", found)
		}
	}
}
