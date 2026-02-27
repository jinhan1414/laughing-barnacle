package agent

import "strings"

func (a *Agent) buildToolRuntimePrompt() string {
	switch preferredShellName() {
	case "powershell", "pwsh":
		return strings.TrimSpace(
			"工具执行环境：linux__bash 当前在 Windows PowerShell 下执行。\n" +
				"linux__bash 仅保留 command 参数，命令内容直接写在 command 字段（不使用 cmd/timeout_sec/working_dir）。\n" +
				"若需调用 curl，请使用 curl.exe（避免命中 PowerShell 的 curl 别名）。\n" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。\n" +
				"维护写接口默认使用 JSON：MCP 用 /api/mcp/services/save|toggle|delete，Skills 用 /api/skills/save|toggle|delete|install，Schedules 用 /api/schedules/save|toggle|delete|run。\n" +
				"维护写入必须带 Content-Type: application/json；禁止把 --data-urlencode 作为默认写入方式。\n" +
				"保存定时任务时，/api/schedules/save 必填字段固定为：id,name,description,action=skill:<skill_id>,cron_expr,enabled（禁止使用 cron/prompt/action=reminder）。\n" +
				"A2A 接入维护统一使用 JSON 接口：POST /api/a2a/agents/save、/api/a2a/agents/toggle、/api/a2a/agents/delete；查询使用 GET /api/a2a/agents（详情 /api/a2a/agents/read?id=<agent_id>）。\n" +
				"A2A JSON 写入优先使用 Invoke-RestMethod + ConvertTo-Json，避免在命令行手写多层转义。\n" +
				"A2A 任务执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel；禁止使用 a2a__send/get/cancel。",
		)
	case "cmd":
		return strings.TrimSpace(
			"工具执行环境：linux__bash 当前在 Windows cmd 下执行。\n" +
				"linux__bash 仅保留 command 参数，命令内容直接写在 command 字段（不使用 cmd/timeout_sec/working_dir）。\n" +
				"写 URL 时使用正常双引号，禁止写反斜杠转义引号（如 \\\"http://...\\\"）。\n" +
				"只能使用 cmd 兼容命令（如 curl、dir、findstr、schtasks、echo），禁止使用 Linux 命令（如 ls/find/head/grep/uname/crontab）。\n" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。\n" +
				"维护写接口默认使用 JSON：MCP 用 /api/mcp/services/save|toggle|delete，Skills 用 /api/skills/save|toggle|delete|install，Schedules 用 /api/schedules/save|toggle|delete|run。\n" +
				"维护写入必须带 -H \"Content-Type: application/json\"，并使用 JSON body（不要默认使用 --data-urlencode）。\n" +
				"保存定时任务时，/api/schedules/save 必填字段固定为：id,name,description,action=skill:<skill_id>,cron_expr,enabled（禁止使用 cron/prompt/action=reminder）。\n" +
				"A2A 接入维护统一使用 JSON 接口：POST /api/a2a/agents/save、/api/a2a/agents/toggle、/api/a2a/agents/delete；查询使用 GET /api/a2a/agents（详情 /api/a2a/agents/read?id=<agent_id>）。\n" +
				"A2A JSON 写入必须带 -H \"Content-Type: application/json\"，并使用单条 -d \"{\\\"key\\\":...}\"。\n" +
				"A2A 任务执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel；禁止使用 a2a__send/get/cancel。",
		)
	case "bash", "sh":
		return strings.TrimSpace(
			"工具执行环境：linux__bash 当前在 Linux shell 下执行。\n" +
				"linux__bash 仅保留 command 参数，命令内容直接写在 command 字段（不使用 cmd/timeout_sec/working_dir）。\n" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。\n" +
				"维护写接口默认使用 JSON：MCP 用 /api/mcp/services/save|toggle|delete，Skills 用 /api/skills/save|toggle|delete|install，Schedules 用 /api/schedules/save|toggle|delete|run。\n" +
				"维护写入必须带 Content-Type: application/json；不要默认使用 --data-urlencode。\n" +
				"保存定时任务时，/api/schedules/save 必填字段固定为：id,name,description,action=skill:<skill_id>,cron_expr,enabled（禁止使用 cron/prompt/action=reminder）。\n" +
				"A2A 接入维护统一使用 JSON 接口：POST /api/a2a/agents/save、/api/a2a/agents/toggle、/api/a2a/agents/delete；查询使用 GET /api/a2a/agents（详情 /api/a2a/agents/read?id=<agent_id>）。\n" +
				"A2A 任务执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel；禁止使用 a2a__send/get/cancel。",
		)
	default:
		return strings.TrimSpace(
			"调用 linux__bash 时仅保留 command 参数，命令内容直接写在 command 字段（不使用 cmd/timeout_sec/working_dir）；" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）；" +
				"维护写接口默认使用 JSON：/api/mcp/services/save|toggle|delete、/api/skills/save|toggle|delete|install、/api/schedules/save|toggle|delete|run；" +
				"维护写入必须使用 Content-Type: application/json；" +
				"保存定时任务时，/api/schedules/save 必填字段固定为 id,name,description,action=skill:<skill_id>,cron_expr,enabled；" +
				"A2A 接入维护统一使用 JSON 接口 /api/a2a/agents/save|toggle|delete，查询使用 /api/a2a/agents 与 /api/a2a/agents/read?id=<agent_id>；" +
				"A2A 任务执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel，禁止使用 a2a__send|get|cancel。",
		)
	}
}
