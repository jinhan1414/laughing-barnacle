package agent

import (
	"laughing-barnacle/internal/conversation"
	"strconv"
	"strings"
)

func shouldRequireRuntimeToolEvidence(userInput string) bool {
	text := strings.ToLower(strings.TrimSpace(userInput))
	if text == "" {
		return false
	}

	domainHits := []string{"定时任务", "schedule", "cron", "mcp", "skill", "memory", "记忆", "服务配置", "api/schedules", "api/skills", "api/memory"}
	hasDomain := false
	for _, token := range domainHits {
		if strings.Contains(text, token) {
			hasDomain = true
			break
		}
	}
	if !hasDomain {
		return false
	}

	intentHits := []string{
		"有哪些", "列表", "当前", "状态", "查看", "查询",
		"启用", "禁用", "删除", "移除", "新增", "创建", "修改", "更新",
		"运行", "执行", "触发", "run", "enable", "disable", "delete",
	}
	for _, token := range intentHits {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func needsExecutionClaimCorrection(reply string, executedCalls []conversation.ToolCall) bool {
	if !containsMutationSuccessClaim(reply) {
		return false
	}
	if containsExecutionFailureCue(reply) {
		return true
	}

	evidence := analyzeExecutionEvidence(executedCalls)
	if evidence.writeSuccess <= 0 {
		return true
	}
	if evidence.writeFailure > 0 {
		return true
	}
	if !evidence.readbackAfterWrite {
		return true
	}
	return false
}

type executionEvidence struct {
	writeAttempts      int
	writeSuccess       int
	writeFailure       int
	readbackAfterWrite bool
}

func analyzeExecutionEvidence(executedCalls []conversation.ToolCall) executionEvidence {
	out := executionEvidence{}
	writeSeen := false
	for _, call := range executedCalls {
		command, ok := extractLinuxBashCommand(call)
		if !ok {
			continue
		}
		isWrite := isLikelyMutationCommand(command)
		isReadback := isLikelyReadbackCommand(command)
		failed := toolCallFailed(call)
		if isWrite {
			writeSeen = true
			out.writeAttempts++
			if failed {
				out.writeFailure++
			} else {
				out.writeSuccess++
			}
			continue
		}
		if writeSeen && isReadback {
			out.readbackAfterWrite = true
		}
	}
	return out
}

func extractLinuxBashCommand(call conversation.ToolCall) (string, bool) {
	if strings.TrimSpace(call.Name) != builtinLinuxBashToolName {
		return "", false
	}
	command, err := parseCommandToolArgument(call.Arguments)
	if err != nil {
		return "", false
	}
	command = strings.ToLower(strings.TrimSpace(command))
	return command, command != ""
}

func toolCallFailed(call conversation.ToolCall) bool {
	if strings.TrimSpace(call.Error) != "" {
		return true
	}
	result := strings.TrimSpace(call.Result)
	if result == "" {
		return false
	}
	lower := strings.ToLower(result)
	if strings.Contains(lower, "tool execution error") {
		return true
	}
	if exitCode, ok := parseToolExitCode(result); ok {
		return exitCode != 0
	}
	return false
}

func parseToolExitCode(result string) (int, bool) {
	for _, line := range strings.Split(strings.ReplaceAll(result, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "exit_code:") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), "exit_code:"))
		if raw == "" {
			return 0, false
		}
		code, err := strconv.Atoi(raw)
		if err != nil {
			return 0, false
		}
		return code, true
	}
	return 0, false
}

func isLikelyMutationCommand(command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" || !strings.Contains(command, "curl") {
		return false
	}
	if strings.Contains(command, "-x post") || strings.Contains(command, "--request post") {
		return true
	}
	if strings.Contains(command, "/settings/") {
		return true
	}
	if strings.Contains(command, "/api/memory/upsert") || strings.Contains(command, "/api/memory/move") || strings.Contains(command, "/api/memory/delete") {
		return true
	}
	return false
}

func isLikelyReadbackCommand(command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" || !strings.Contains(command, "curl") {
		return false
	}
	readPaths := []string{
		"/api/skills",
		"/api/schedules",
		"/api/mcp/services",
		"/api/memory/index",
		"/api/memory/read",
		"/api/memory/section",
	}
	for _, path := range readPaths {
		if strings.Contains(command, path) {
			return true
		}
	}
	return false
}

func containsMutationSuccessClaim(reply string) bool {
	text := strings.TrimSpace(reply)
	if text == "" {
		return false
	}
	successMarkers := []string{
		"已成功创建", "创建成功", "已创建", "创建完成",
		"已成功新增", "新增成功", "已新增", "添加成功", "已添加",
		"已成功修改", "修改成功", "已修改",
		"已成功更新", "更新成功", "已更新",
		"已成功启用", "启用成功", "已启用",
		"已成功禁用", "禁用成功", "已禁用",
		"已成功删除", "删除成功", "已删除",
		"已成功保存", "保存成功", "已保存",
		"已成功安装", "安装成功", "已安装",
		"已生效", "已配置",
	}
	for _, marker := range successMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func containsExecutionFailureCue(reply string) bool {
	text := strings.TrimSpace(reply)
	if text == "" {
		return false
	}
	failureMarkers := []string{
		"未成功", "失败", "未出现", "未返回", "未生效", "无法", "错误",
		"不存在", "缺失", "未完成", "没成功", "没有成功",
	}
	for _, marker := range failureMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
