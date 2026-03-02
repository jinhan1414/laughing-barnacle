package agent

import (
	"fmt"
	"strings"

	"laughing-barnacle/internal/llm"
)

func appendNearToolBudgetPrompt(messages []llm.Message, toolRounds, maxRounds int) []llm.Message {
	prompt := nearToolBudgetPrompt(toolRounds, maxRounds)
	if prompt == "" {
		return messages
	}
	return append(messages, llm.Message{Role: "system", Content: prompt})
}

func nearToolBudgetPrompt(toolRounds, maxRounds int) string {
	if maxRounds <= 1 || toolRounds != maxRounds-1 {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf(
		"工具调用预算即将耗尽：当前已执行 %d/%d 轮，本轮再扩张本地执行步骤后，下一次将无法继续调用工具。\n"+
			"若当前任务仍属于大型开发、仓库分析或需要多步后台续跑，优先委托最相关的已启用 A2A agent 执行主体工作。\n"+
			"如需后台继续，必须在本轮先调用 async_task__submit，再调用 autonomous_run__checkpoint 写入 waiting_async 与 waiting_ref；数字分身仅负责调度、回读和汇总。",
		toolRounds, maxRounds,
	))
}
