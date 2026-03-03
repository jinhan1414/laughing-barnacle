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
	if a.runs != nil {
		builtinToolDefs = append(builtinToolDefs, autonomousRunBuiltinToolDefinitions()...)
	}
	skillIndexPrompt := ""
	memoryIndexPrompt := ""
	projectIndexPrompt := ""
	a2aIndexPrompt := ""
	asyncTaskIndexPrompt := ""
	runIndexPrompt := ""
	resourceHeaderInjected := false
	resourceSection := 0
	if a.skills != nil {
		allSkillIndex := a.skills.ListEnabledSkillIndex()
		if len(allSkillIndex) > 0 {
			requestMessages = appendResourceIndexHeader(requestMessages, &resourceHeaderInjected)
			resourceSection++
			skillIndexPrompt = buildSkillIndexPrompt(allSkillIndex, resourceSection)
			if skillIndexPrompt != "" {
				requestMessages = append(requestMessages, llm.Message{
					Role:    "system",
					Content: skillIndexPrompt,
				})
			}
		}
	}
	if a.memory != nil {
		projectIndex := a.listProjectIndexLines(20)
		if len(projectIndex) > 0 {
			requestMessages = appendResourceIndexHeader(requestMessages, &resourceHeaderInjected)
			resourceSection++
			projectIndexPrompt = buildProjectIndexPrompt(projectIndex, resourceSection)
			if projectIndexPrompt != "" {
				requestMessages = append(requestMessages, llm.Message{
					Role:    "system",
					Content: projectIndexPrompt,
				})
			}
		}
		memoryIndex := a.memory.ListIndexLines(20)
		if len(memoryIndex) > 0 {
			requestMessages = appendResourceIndexHeader(requestMessages, &resourceHeaderInjected)
			resourceSection++
			memoryIndexPrompt = buildMemoryIndexPrompt(memoryIndex, resourceSection)
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
			requestMessages = appendResourceIndexHeader(requestMessages, &resourceHeaderInjected)
			resourceSection++
			a2aIndexPrompt = buildA2AIndexPrompt(a2aIndex, resourceSection)
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
			requestMessages = appendResourceIndexHeader(requestMessages, &resourceHeaderInjected)
			resourceSection++
			asyncTaskIndexPrompt = buildAsyncTaskIndexPrompt(asyncIndex, resourceSection)
			if asyncTaskIndexPrompt != "" {
				requestMessages = append(requestMessages, llm.Message{
					Role:    "system",
					Content: asyncTaskIndexPrompt,
				})
			}
		}
	}
	if a.runs != nil {
		runIndex := a.runs.ListIndexLines(maxAutonomousRunIndexLines, a.nowFn())
		if len(runIndex) > 0 {
			requestMessages = appendResourceIndexHeader(requestMessages, &resourceHeaderInjected)
			resourceSection++
			runIndexPrompt = buildRunIndexPrompt(runIndex, resourceSection)
			if runIndexPrompt != "" {
				requestMessages = append(requestMessages, llm.Message{
					Role:    "system",
					Content: runIndexPrompt,
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
	summary = pruneSummaryOverlap(summary, systemPrompt, skillIndexPrompt, projectIndexPrompt, memoryIndexPrompt, a2aIndexPrompt, asyncTaskIndexPrompt, runIndexPrompt, runtimePrompt)
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
		requestMessages = appendNearToolBudgetPrompt(requestMessages, toolRounds, maxRounds)
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
			if stop, guardPrompt := shouldStopToolLoopOnError(callName, callErr); stop {
				requestMessages = append(requestMessages, llm.Message{
					Role:    "system",
					Content: guardPrompt,
				})
				finalContent, finalCalls, finalUsage, finalErr := finalWithoutTools()
				if finalErr != nil {
					return "", finalCalls, finalUsage, finalErr
				}
				return finalContent, finalCalls, finalUsage, nil
			}
		}
	}
}
