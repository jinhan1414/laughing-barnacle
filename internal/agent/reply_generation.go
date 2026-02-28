package agent

import (
	"context"
	"fmt"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
	"strings"
)

func (a *Agent) generateReply(ctx context.Context, messages []conversation.Message) (string, []conversation.ToolCall, *conversation.TokenUsage, error) {
	summary, _ := a.store.Snapshot()
	systemPrompt, _ := a.resolvePromptsLocked()

	requestMessages := make([]llm.Message, 0, 2+len(messages))
	requestMessages = append(requestMessages, llm.Message{
		Role:    "system",
		Content: systemPrompt,
	})
	builtinToolDefs := []llm.ToolDefinition{
		linuxBashToolDefinition(),
		contextReadToolDefinition(),
		maintenanceWriteToolDefinition(),
	}
	if a.asyncTasks != nil {
		builtinToolDefs = append(builtinToolDefs, asyncTaskBuiltinToolDefinitions()...)
	}
	skillIndexPrompt := ""
	memoryIndexPrompt := ""
	a2aIndexPrompt := ""
	asyncTaskIndexPrompt := ""
	resourceHeaderInjected := false
	resourceSection := 0
	if a.skills != nil {
		allSkillIndex := a.skills.ListEnabledSkillIndex()
		if len(allSkillIndex) > 0 {
			var b strings.Builder
			if !resourceHeaderInjected {
				b.WriteString("# Resource Indexes (资源索引 - 渐进式披露)\n")
				resourceHeaderInjected = true
			}
			resourceSection++
			b.WriteString(fmt.Sprintf("## %d. Skills 索引 (共 %d 条)\n", resourceSection, len(allSkillIndex)))
			b.WriteString("如需技能详情，仅在必要时调用：context__read(resource=\"skills\", action=\"read\", id=\"<skill_id>\")。\n")
			b.WriteString("Skill 调用规则：每轮先按用户请求语义判断是否命中某个 skill_id；一旦命中，无需用户点名，先读取该 skill 详情再执行。\n")
			b.WriteString("为节省上下文，单轮默认只读取 1 个最相关技能；若仍不足，再按需补充读取。\n")
			b.WriteString("若未命中或技能不适用，再按普通问答流程回复。\n")

			injectCandidates := compactSkillIndexByIDs(allSkillIndex, nil)
			injected := 0
			for _, line := range injectCandidates {
				if line == "" {
					continue
				}
				b.WriteString(fmt.Sprintf("%d. %s\n", injected+1, line))
				injected++
			}
			skillIndexPrompt = strings.TrimSpace(b.String())
			if skillIndexPrompt != "" {
				requestMessages = append(requestMessages, llm.Message{
					Role:    "system",
					Content: skillIndexPrompt,
				})
			}
		}
	}
	if a.memory != nil {
		memoryIndex := a.memory.ListIndexLines(20)
		if len(memoryIndex) > 0 {
			var b strings.Builder
			if !resourceHeaderInjected {
				b.WriteString("# Resource Indexes (资源索引 - 渐进式披露)\n")
				resourceHeaderInjected = true
			}
			resourceSection++
			b.WriteString(fmt.Sprintf("## %d. MemoryFS 索引 (共 %d 条)\n", resourceSection, len(memoryIndex)))
			b.WriteString(fmt.Sprintf("MemoryFS 记忆索引（渐进式披露）：共 %d 条。\n", len(memoryIndex)))
			b.WriteString("读取规则：先读索引，再按需读取文件摘要和分节，禁止一次性拉取全文。\n")
			b.WriteString("按需读取：context__read(resource=\"memory\", action=\"read\", path=\"<path>\")。\n")
			b.WriteString("按需分节：context__read(resource=\"memory\", action=\"section\", path=\"<path>\", section_id=\"<id>\")。\n")
			for i, line := range memoryIndex {
				line = trimRunes(strings.TrimSpace(line), maxSingleSkillPromptRunes)
				if line == "" {
					continue
				}
				b.WriteString(fmt.Sprintf("%d. %s\n", i+1, line))
			}
			memoryIndexPrompt = strings.TrimSpace(b.String())
			if memoryIndexPrompt != "" {
				requestMessages = append(requestMessages, llm.Message{
					Role:    "system",
					Content: memoryIndexPrompt,
				})
			}
		}
	}
	if a.a2a != nil {
		a2aIndex := a.a2a.ListIndexLines(20)
		if len(a2aIndex) > 0 {
			var b strings.Builder
			if !resourceHeaderInjected {
				b.WriteString("# Resource Indexes (资源索引 - 渐进式披露)\n")
				resourceHeaderInjected = true
			}
			resourceSection++
			b.WriteString(fmt.Sprintf("## %d. A2A Agents 索引 (共 %d 条)\n", resourceSection, len(a2aIndex)))
			b.WriteString("读取规则：本索引是本轮固定上下文主来源，先基于索引选择 agent_id；禁止一次性拉取全部 AgentCard 正文。\n")
			b.WriteString("按需读取详情：context__read(resource=\"a2a\", action=\"read\", id=\"<agent_id>\")。\n")
			b.WriteString("仅在明确需要刷新列表或执行前做一致性校验时，才读取列表：context__read(resource=\"a2a\", action=\"list\")。\n")
			b.WriteString("单轮默认只读取 1 个最相关 Agent 详情；若仍不足，再按需补读。\n")
			for i, line := range a2aIndex {
				line = trimRunes(strings.TrimSpace(line), maxSingleSkillPromptRunes)
				if line == "" {
					continue
				}
				b.WriteString(fmt.Sprintf("%d. %s\n", i+1, line))
			}
			a2aIndexPrompt = strings.TrimSpace(b.String())
			if a2aIndexPrompt != "" {
				requestMessages = append(requestMessages, llm.Message{
					Role:    "system",
					Content: a2aIndexPrompt,
				})
			}
		}
	}
	if a.asyncTasks != nil {
		asyncIndex := a.asyncTasks.ListIndexLines(20, a.nowFn())
		if len(asyncIndex) > 0 {
			var b strings.Builder
			if !resourceHeaderInjected {
				b.WriteString("# Resource Indexes (资源索引 - 渐进式披露)\n")
				resourceHeaderInjected = true
			}
			resourceSection++
			b.WriteString(fmt.Sprintf("## %d. Active Async Tasks 索引 (共 %d 条)\n", resourceSection, len(asyncIndex)))
			b.WriteString(fmt.Sprintf("后台任务索引（固定注入，当天+运行中）：共 %d 条。\n", len(asyncIndex)))
			b.WriteString("执行规则：任务发起统一使用 async_task__submit；查询与取消使用 async_task__get/cancel。\n")
			b.WriteString("参数约束：submit 必填 task_type,request；task_type=a2a 时必须提供 agent_id,agent_input。\n")
			b.WriteString("详情读取规则：默认通过 context__read(resource=\"async\", action=\"get\", task_id=\"<task_id>\") 读取；仅在必要时附加 include_logs/log_cursor/log_limit。\n")
			for i, line := range asyncIndex {
				line = trimRunes(strings.TrimSpace(line), maxSingleSkillPromptRunes)
				if line == "" {
					continue
				}
				b.WriteString(fmt.Sprintf("%d. %s\n", i+1, line))
			}
			asyncTaskIndexPrompt = strings.TrimSpace(b.String())
			if asyncTaskIndexPrompt != "" {
				requestMessages = append(requestMessages, llm.Message{
					Role:    "system",
					Content: asyncTaskIndexPrompt,
				})
			}
		}
	}
	runtimePrompt := strings.TrimSpace(a.buildToolRuntimePrompt())
	if runtimePrompt != "" {
		requestMessages = append(requestMessages, llm.Message{
			Role:    "system",
			Content: runtimePrompt,
		})
	}
	currentDateContextPrompt := strings.TrimSpace(buildCurrentDateUserContextPrompt(a.nowFn()))
	if currentDateContextPrompt != "" {
		requestMessages = append(requestMessages, llm.Message{
			Role:    "system",
			Content: currentDateContextPrompt,
		})
	}
	summary = pruneSummaryOverlap(summary, systemPrompt, skillIndexPrompt, memoryIndexPrompt, a2aIndexPrompt, asyncTaskIndexPrompt, runtimePrompt)
	if strings.TrimSpace(summary) != "" {
		requestMessages = append(requestMessages, llm.Message{
			Role:    "system",
			Content: "历史摘要（由系统自动压缩）：\n" + trimRunes(summary, maxSummaryForRequestRunes),
		})
	}

	recentMessages := trimMessagesForRequest(messages, a.cfg.MaxRecentMessages, maxRecentContextRunes, maxContextMessageRunes)
	requestMessages = appendHistoryMessagesWithToolCalls(requestMessages, recentMessages)
	requestMessages = removeRuntimeDateContextUserMessages(requestMessages)

	toolDefs := make([]llm.ToolDefinition, 0, len(builtinToolDefs)+4)
	toolDefs = append(toolDefs, builtinToolDefs...)
	if a.tools != nil {
		externalDefs, err := a.tools.ListTools(ctx)
		if err == nil {
			toolDefs = append(toolDefs, externalDefs...)
		}
	}

	if len(toolDefs) == 0 {
		resp, err := a.llm.Chat(ctx, llm.ChatRequest{
			Purpose:     "chat_reply",
			Model:       a.cfg.Model,
			Messages:    requestMessages,
			Temperature: a.cfg.Temperature,
		})
		if err != nil {
			return "", nil, nil, fmt.Errorf("generate reply failed: %w", err)
		}
		return resp.Content, nil, toConversationUsage(resp.Usage), nil
	}

	maxRounds := a.cfg.MaxToolCallRounds
	if maxRounds <= 0 {
		maxRounds = 1
	}
	executedCalls := make([]conversation.ToolCall, 0)
	usageTotal := conversation.TokenUsage{}
	hasUsage := false
	finalWithoutTools := func() (string, []conversation.ToolCall, *conversation.TokenUsage, error) {
		finalResp, finalErr := a.llm.Chat(ctx, llm.ChatRequest{
			Purpose:     "chat_reply",
			Model:       a.cfg.Model,
			Messages:    requestMessages,
			Temperature: a.cfg.Temperature,
		})
		if finalErr != nil {
			return "", executedCalls, usageOrNil(usageTotal, hasUsage), fmt.Errorf("generate reply failed: %w", finalErr)
		}
		usageTotal, hasUsage = mergeTokenUsage(usageTotal, hasUsage, finalResp.Usage)
		return finalResp.Content, executedCalls, usageOrNil(usageTotal, hasUsage), nil
	}

	toolRounds := 0
	for {
		resp, err := a.llm.Chat(ctx, llm.ChatRequest{
			Purpose:     "chat_reply",
			Model:       a.cfg.Model,
			Messages:    requestMessages,
			Tools:       toolDefs,
			Temperature: a.cfg.Temperature,
		})
		if err != nil {
			return "", executedCalls, usageOrNil(usageTotal, hasUsage), fmt.Errorf("generate reply failed: %w", err)
		}
		usageTotal, hasUsage = mergeTokenUsage(usageTotal, hasUsage, resp.Usage)

		if len(resp.ToolCalls) == 0 {
			return resp.Content, executedCalls, usageOrNil(usageTotal, hasUsage), nil
		}
		if toolRounds >= maxRounds {
			requestMessages = append(requestMessages, llm.Message{
				Role:    "system",
				Content: fmt.Sprintf("已达到工具调用上限（%d 轮）。禁止继续调用工具，请仅基于已有对话与工具结果直接回答；若信息不足，请明确说明缺口。", maxRounds),
			})
			finalContent, finalCalls, finalUsage, finalErr := finalWithoutTools()
			if finalErr != nil {
				return "", finalCalls, finalUsage, finalErr
			}
			if strings.TrimSpace(finalContent) == "" {
				return "", finalCalls, finalUsage, fmt.Errorf("tool call rounds exceeded %d", maxRounds)
			}
			return finalContent, finalCalls, finalUsage, nil
		}
		toolRounds++

		requestMessages = append(requestMessages, llm.Message{
			Role:      "assistant",
			Content:   trimRunes(resp.Content, maxContextMessageRunes),
			ToolCalls: resp.ToolCalls,
		})

		for _, call := range resp.ToolCalls {
			result, callErr := a.callTool(ctx, call)
			if callErr != nil {
				result = "tool execution error: " + callErr.Error()
			}
			callName := strings.TrimSpace(call.Function.Name)
			if callName == "" {
				callName = "(unknown)"
			}
			callArgs := strings.TrimSpace(call.Function.Arguments)
			if callArgs == "" {
				callArgs = "{}"
			}
			trimmedResult := strings.TrimSpace(trimRunes(result, maxContextMessageRunes))
			callRecord := conversation.ToolCall{
				ID:        strings.TrimSpace(call.ID),
				Name:      callName,
				Arguments: callArgs,
				Result:    trimmedResult,
				CreatedAt: a.nowFn(),
			}
			if callErr != nil {
				callRecord.Error = callErr.Error()
			}
			executedCalls = append(executedCalls, callRecord)

			toolCallID := strings.TrimSpace(call.ID)
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("tool_call_%d_%s", toolRounds, call.Function.Name)
			}
			requestMessages = append(requestMessages, llm.Message{
				Role:       "tool",
				ToolCallID: toolCallID,
				Content:    trimmedResult,
			})
		}
	}
}
