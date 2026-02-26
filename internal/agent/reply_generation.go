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
	localAPIBaseURL := a.localAPIBaseURL()

	requestMessages := make([]llm.Message, 0, 2+len(messages))
	requestMessages = append(requestMessages, llm.Message{
		Role:    "system",
		Content: systemPrompt,
	})
	responseStylePrompt := "回答策略：默认简洁直答（3-6 行）。仅当用户明确要求“详细/方案/步骤/复盘/计划/总结”时再展开，避免无关模板、表格和冗长铺垫。"
	builtinToolDefs := []llm.ToolDefinition{linuxBashToolDefinition()}
	if a.a2a != nil {
		builtinToolDefs = append(builtinToolDefs, a2aBuiltinToolDefinitions()...)
	}
	skillIndexPrompt := ""
	memoryIndexPrompt := ""
	a2aIndexPrompt := ""
	if a.skills != nil {
		allSkillIndex := a.skills.ListEnabledSkillIndex()
		if len(allSkillIndex) > 0 {
			var b strings.Builder
			b.WriteString(fmt.Sprintf("已启用技能索引（渐进式披露）：共 %d 条。\n", len(allSkillIndex)))
			b.WriteString(fmt.Sprintf("如需技能详情，仅在必要时通过 linux__bash 执行：curl -s \"%s/api/skills/read?id=<skill_id>\"。\n", localAPIBaseURL))
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
			b.WriteString(fmt.Sprintf("MemoryFS 记忆索引（渐进式披露）：共 %d 条。\n", len(memoryIndex)))
			b.WriteString("读取规则：先读索引，再按需读取文件摘要和分节，禁止一次性拉取全文。\n")
			b.WriteString(fmt.Sprintf("按需读取：curl -s \"%s/api/memory/read?path=<path>\"。\n", localAPIBaseURL))
			b.WriteString(fmt.Sprintf("按需分节：curl -s \"%s/api/memory/section?path=<path>&section_id=<id>\"。\n", localAPIBaseURL))
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
			b.WriteString(fmt.Sprintf("A2A 已接入 Agent 索引（渐进式披露）：共 %d 条。\n", len(a2aIndex)))
			b.WriteString("读取规则：本索引是本轮固定上下文主来源，先基于索引选择 agent_id；禁止一次性拉取全部 AgentCard 正文。\n")
			b.WriteString(fmt.Sprintf("按需读取详情：curl -s \"%s/api/a2a/agents/read?id=<agent_id>\"。\n", localAPIBaseURL))
			b.WriteString(fmt.Sprintf("仅在明确需要刷新列表或执行前做一致性校验时，才读取列表：curl -s \"%s/api/a2a/agents\"。\n", localAPIBaseURL))
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
	requestMessages = append(requestMessages, llm.Message{
		Role:    "system",
		Content: responseStylePrompt,
	})
	runtimePrompt := strings.TrimSpace(a.buildToolRuntimePrompt())
	if runtimePrompt != "" {
		requestMessages = append(requestMessages, llm.Message{
			Role:    "system",
			Content: runtimePrompt,
		})
	}
	summary = pruneSummaryOverlap(summary, systemPrompt, skillIndexPrompt, memoryIndexPrompt, a2aIndexPrompt, responseStylePrompt, runtimePrompt)
	if strings.TrimSpace(summary) != "" {
		requestMessages = append(requestMessages, llm.Message{
			Role:    "system",
			Content: "历史摘要（由系统自动压缩）：\n" + trimRunes(summary, maxSummaryForRequestRunes),
		})
	}

	recentMessages := trimMessagesForRequest(messages, a.cfg.MaxRecentMessages, maxRecentContextRunes, maxContextMessageRunes)
	requestMessages = appendHistoryMessagesWithToolCalls(requestMessages, recentMessages)
	latestUserInput := latestUserMessageText(recentMessages)
	requiresToolEvidence := shouldRequireRuntimeToolEvidence(latestUserInput)
	requestMessages = removeRuntimeDateContextUserMessages(requestMessages)
	requestMessages = append(requestMessages, llm.Message{
		Role:    "user",
		Content: buildCurrentDateUserContextPrompt(a.nowFn()),
	})

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
	enforcedEvidence := false
	enforcedExecutionClaimValidation := false

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
			if requiresToolEvidence && len(executedCalls) == 0 {
				if !enforcedEvidence {
					enforcedEvidence = true
					requestMessages = append(requestMessages, llm.Message{
						Role:    "system",
						Content: "上一条回答缺少工具证据。针对此类“实时状态/配置变更”问题，必须先调用工具查询或执行，再基于工具结果回复。",
					})
					continue
				}
				return "", executedCalls, usageOrNil(usageTotal, hasUsage), fmt.Errorf("需要先调用工具获取实时数据后再回答")
			}
			if needsExecutionClaimCorrection(resp.Content, executedCalls) {
				if !enforcedExecutionClaimValidation {
					enforcedExecutionClaimValidation = true
					requestMessages = append(requestMessages, llm.Message{
						Role: "system",
						Content: "上一条回答包含未验证或自相矛盾的执行结论。只有在工具结果显示“写操作成功且已回读验证”后，才能声称“已创建/已修改/已启用/已删除”。" +
							"若未满足条件，必须明确说明未完成，并继续调用工具完成执行与校验。",
					})
					continue
				}
				return "", executedCalls, usageOrNil(usageTotal, hasUsage), fmt.Errorf("回答包含未验证的执行成功结论，请先调用工具并基于结果回答")
			}
			return resp.Content, executedCalls, usageOrNil(usageTotal, hasUsage), nil
		}
		if toolRounds >= maxRounds {
			requestMessages = append(requestMessages, llm.Message{
				Role:    "system",
				Content: fmt.Sprintf("已达到工具调用上限（%d 轮）。禁止继续调用工具，请仅基于已有对话与工具结果直接回答；若信息不足，请明确说明缺口。", maxRounds),
			})
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
			if strings.TrimSpace(finalResp.Content) == "" {
				return "", executedCalls, usageOrNil(usageTotal, hasUsage), fmt.Errorf("tool call rounds exceeded %d", maxRounds)
			}
			return finalResp.Content, executedCalls, usageOrNil(usageTotal, hasUsage), nil
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
