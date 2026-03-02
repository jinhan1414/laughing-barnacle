package agent

import (
	"fmt"
	"strings"

	"laughing-barnacle/internal/llm"
)

func appendResourceIndexHeader(messages []llm.Message, injected *bool) []llm.Message {
	if injected == nil || *injected {
		return messages
	}
	*injected = true
	return append(messages, llm.Message{
		Role:    "system",
		Content: "# Resource Indexes (资源索引 - 渐进式披露)",
	})
}

func buildMemoryIndexPrompt(lines []string, section int) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## %d. MemoryFS 索引 (共 %d 条)\n", section, len(lines)))
	b.WriteString(fmt.Sprintf("MemoryFS 记忆索引（渐进式披露）：共 %d 条。\n", len(lines)))
	b.WriteString("读取规则：先读索引，再按需读取文件摘要和分节，禁止一次性拉取全文。\n")
	b.WriteString("按需读取：context__read(resource=\"memory\", action=\"read\", path=\"<path>\")。\n")
	b.WriteString("按需分节：context__read(resource=\"memory\", action=\"section\", path=\"<path>\", section_id=\"<id>\")。\n")
	appendIndexLines(&b, lines)
	return strings.TrimSpace(b.String())
}

func buildA2AIndexPrompt(lines []string, section int) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## %d. A2A Agents 索引 (共 %d 条)\n", section, len(lines)))
	b.WriteString("读取规则：本索引是本轮固定上下文主来源，先基于索引选择 agent_id；禁止一次性拉取全部 AgentCard 正文。\n")
	b.WriteString("默认策略：大型开发、仓库分析、多步产出类任务，若索引中存在匹配且 enabled=true 的 Agent，优先委托该 Agent 执行主体工作；数字分身负责调度、回读与汇总。\n")
	b.WriteString("按需读取详情：context__read(resource=\"a2a\", action=\"read\", id=\"<agent_id>\")。\n")
	b.WriteString("仅在明确需要刷新列表或执行前做一致性校验时，才读取列表：context__read(resource=\"a2a\", action=\"list\")。\n")
	b.WriteString("单轮默认只读取 1 个最相关 Agent 详情；若仍不足，再按需补读。\n")
	appendIndexLines(&b, lines)
	return strings.TrimSpace(b.String())
}

func buildAsyncTaskIndexPrompt(lines []string, section int) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## %d. Active Async Tasks 索引 (共 %d 条)\n", section, len(lines)))
	b.WriteString(fmt.Sprintf("后台任务索引（固定注入，当天+运行中）：共 %d 条。\n", len(lines)))
	b.WriteString("执行规则：任务发起统一使用 async_task__submit；查询与取消使用 async_task__get/cancel。\n")
	b.WriteString("参数约束：submit 必填 task_type,request；task_type=a2a 时必须提供 agent_id,agent_input。\n")
	b.WriteString("详情读取规则：默认通过 context__read(resource=\"async\", action=\"get\", task_id=\"<task_id>\") 读取；仅在必要时附加 include_logs/log_cursor/log_limit。\n")
	appendIndexLines(&b, lines)
	return strings.TrimSpace(b.String())
}

func buildRunIndexPrompt(lines []string, section int) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## %d. Autonomous Runs 索引 (共 %d 条)\n", section, len(lines)))
	b.WriteString("自主运行索引（固定注入，当天+活动中）：用于跟踪多步自动目标。\n")
	b.WriteString("读取规则：默认只看 run 索引；需要详情时再用 context__read(resource=\"runs\", action=\"get\", id=\"<run_id>\")。\n")
	b.WriteString("若任务需要在后台结果返回后继续自动执行，必须调用 autonomous_run__checkpoint 记录 run 状态。\n")
	appendIndexLines(&b, lines)
	return strings.TrimSpace(b.String())
}

func appendIndexLines(b *strings.Builder, lines []string) {
	for i, line := range lines {
		line = trimRunes(strings.TrimSpace(line), maxSingleSkillPromptRunes)
		if line == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, line))
	}
}
