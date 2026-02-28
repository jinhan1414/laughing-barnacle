package agent

import (
	"context"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"strings"
	"testing"
	"time"
)

func TestHandleUserMessage_IncludesA2AIndexProgressiveDisclosure(t *testing.T) {
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
	agentSvc.SetA2AProvider(&mockA2A{
		indexLines: []string{
			"agent_id=codex-local | name=Codex Local | description=本地编码代理 | status=enabled",
		},
	})

	if _, err := agentSvc.HandleUserMessage(context.Background(), "请帮我查下已接入agent"); err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if len(fakeLLM.calls) != 1 {
		t.Fatalf("expected one llm call, got %d", len(fakeLLM.calls))
	}

	content := ""
	for _, msg := range fakeLLM.calls[0].Messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "A2A Agents 索引 (共") {
			content = msg.Content
			break
		}
	}
	if strings.TrimSpace(content) == "" {
		t.Fatalf("expected a2a index prompt injected")
	}
	if !strings.Contains(content, "context__read(resource=\"a2a\", action=\"read\", id=\"<agent_id>\")") {
		t.Fatalf("expected a2a read hint, got %q", content)
	}
	if !strings.Contains(content, "本索引是本轮固定上下文主来源") {
		t.Fatalf("expected fixed-context hint in a2a index prompt, got %q", content)
	}
	if !strings.Contains(content, "仅在明确需要刷新列表或执行前做一致性校验时") {
		t.Fatalf("expected refresh-on-demand hint in a2a index prompt, got %q", content)
	}
	if strings.Contains(content, "agent_card_json") {
		t.Fatalf("should not inject full agent card content, got %q", content)
	}
}

func TestHandleUserMessage_A2ASubmitRunsViaAsyncTaskTool(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "已转后台处理"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "async_submit_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      builtinAsyncTaskSubmitToolName,
							Arguments: `{"task_type":"a2a","request":"调用codex修复bug","agent_id":"codex-local","agent_input":"请修复这个 bug","notify_on_finish":false}`,
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
	agentSvc.SetA2AProvider(&mockA2A{
		send: A2ATaskResult{
			AgentID: "codex-local",
			TaskID:  "remote-123",
			Status:  "submitted",
		},
		get: A2ATaskResult{
			AgentID: "codex-local",
			TaskID:  "remote-123",
			Status:  "succeeded",
		},
	})

	reply, err := agentSvc.HandleUserMessage(context.Background(), "调用 codex agent 修复 bug")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "已转后台处理" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}
	toolResultFound := false
	for _, msg := range fakeLLM.calls[1].Messages {
		if msg.Role == "tool" &&
			strings.Contains(msg.Content, "async_task_id:") &&
			strings.Contains(msg.Content, "task_type: a2a") {
			toolResultFound = true
			break
		}
	}
	if !toolResultFound {
		t.Fatalf("expected async_task tool result in second llm call")
	}

	_, messages := store.Snapshot()
	if len(messages) != 2 {
		t.Fatalf("expected user + assistant messages, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 1 {
		t.Fatalf("expected one tool call recorded, got %d", len(messages[0].ToolCalls))
	}
	if messages[0].ToolCalls[0].Name != builtinAsyncTaskSubmitToolName {
		t.Fatalf("unexpected tool name: %s", messages[0].ToolCalls[0].Name)
	}
}

func TestHandleUserMessage_A2AInProgressDoesNotBlockCurrentTurn(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "任务已在后台执行"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "async_submit_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      builtinAsyncTaskSubmitToolName,
							Arguments: `{"task_type":"a2a","request":"分析项目","agent_id":"codex-local","agent_input":"请分析项目","notify_on_finish":false}`,
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
		MaxToolCallRounds:          10,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)
	agentSvc.SetA2AProvider(&mockA2A{
		send: A2ATaskResult{
			AgentID: "codex-local",
			TaskID:  "remote-888",
			Status:  "working",
			Message: "submitted",
		},
	})

	start := time.Now()
	reply, err := agentSvc.HandleUserMessage(context.Background(), "用 codex agent 分析项目")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "任务已在后台执行" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("current turn should not block for async polling")
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}
	hasAsyncStatus := false
	for _, msg := range fakeLLM.calls[1].Messages {
		if msg.Role != "tool" || !strings.Contains(msg.Content, "async_task_id:") {
			continue
		}
		if strings.Contains(msg.Content, "status: submitted") || strings.Contains(msg.Content, "status: working") {
			hasAsyncStatus = true
			break
		}
	}
	if !hasAsyncStatus {
		t.Fatalf("expected async task status in tool result")
	}
}

func TestHandleUserMessage_AsyncTaskSubmitValidationErrorReturnsToModel(t *testing.T) {
	store := conversation.NewStore()
	fakeLLM := &mockLLM{
		responses: map[string][]string{
			"chat_reply": {"", "参数不完整，请补充后重试"},
		},
		toolCalls: map[string][][]llm.ToolCall{
			"chat_reply": {
				{
					{
						ID:   "async_submit_1",
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      builtinAsyncTaskSubmitToolName,
							Arguments: `{"task_type":"a2a","request":"查任务","agent_id":"codex-local"}`,
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
		MaxToolCallRounds:          10,
		SystemPrompt:               "system",
		CompressionSystemPrompt:    "compressor",
	}, store, fakeLLM, nil)
	reply, err := agentSvc.HandleUserMessage(context.Background(), "查一下任务状态")
	if err != nil {
		t.Fatalf("HandleUserMessage error: %v", err)
	}
	if reply != "参数不完整，请补充后重试" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if len(fakeLLM.calls) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(fakeLLM.calls))
	}
	toolErrorFound := false
	for _, msg := range fakeLLM.calls[1].Messages {
		if msg.Role == "tool" &&
			strings.Contains(msg.Content, "tool execution error:") &&
			strings.Contains(msg.Content, "agent_id and agent_input are required when task_type=a2a") {
			toolErrorFound = true
			break
		}
	}
	if !toolErrorFound {
		t.Fatalf("expected tool error returned to model in second llm call")
	}
}
