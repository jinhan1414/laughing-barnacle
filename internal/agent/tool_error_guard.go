package agent

import "strings"

const codexLocalProjectContextErr = "codex-local requires metadata.working_dir or metadata.project_id"
const codexLocalWorkingDirRootErr = "metadata.working_dir must be under APP_PROJECT_ROOT_DIR"

func shouldStopToolLoopOnError(callName string, callErr error) (bool, string) {
	if callErr == nil {
		return false, ""
	}
	if !strings.EqualFold(strings.TrimSpace(callName), builtinAsyncTaskSubmitToolName) {
		return false, ""
	}
	if !strings.Contains(callErr.Error(), codexLocalProjectContextErr) {
		if !strings.Contains(callErr.Error(), codexLocalWorkingDirRootErr) {
			return false, ""
		}
		return true, strings.TrimSpace(
			"检测到 async_task__submit 的 working_dir 超出项目根目录约束。" +
				"本轮禁止继续重试提交，请直接提示用户改用 APP_PROJECT_ROOT_DIR 下路径或仅提供 project_id。",
		)
	}
	return true, strings.TrimSpace(
		"检测到 async_task__submit 因缺少项目上下文失败（metadata.working_dir 或 metadata.project_id）。" +
			"本轮禁止继续重试提交，请直接向用户说明缺口并给出下一轮可执行的精确参数。",
	)
}
