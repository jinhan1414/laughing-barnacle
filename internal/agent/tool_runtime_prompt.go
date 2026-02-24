package agent

import "strings"

func (a *Agent) buildToolRuntimePrompt() string {
	switch preferredShellName() {
	case "cmd":
		return strings.TrimSpace(
			"工具执行环境：linux__bash 当前在 Windows cmd 下执行。\n" +
				"调用参数键必须是 command（不是 cmd）。\n" +
				"写 URL 时使用正常双引号，禁止写反斜杠转义引号（如 \\\"http://...\\\"）。\n" +
				"只能使用 cmd 兼容命令（如 curl、dir、findstr、schtasks、echo），禁止使用 Linux 命令（如 ls/find/head/grep/uname/crontab）。\n" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。",
		)
	case "bash", "sh":
		return strings.TrimSpace(
			"工具执行环境：linux__bash 当前在 Linux shell 下执行。\n" +
				"调用参数键必须是 command（不是 cmd）。\n" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。",
		)
	default:
		return strings.TrimSpace(
			"调用 linux__bash 时，参数键必须是 command（不是 cmd）；" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。",
		)
	}
}
