package agent

import "strings"

const executionConsistencyPrompt = "" +
	"执行一致性硬约束（最高优先级）：\n" +
	"涉及执行/查询/分析本地项目/调用工具时，必须先发起工具调用；禁止先输出“我开始执行了/我先跑一下”等承诺语。\n" +
	"若本轮没有任何工具结果（function_call_output），不得声称“已执行/执行中/已完成”。\n" +
	"无法立即执行时，必须明确“我还未执行”，并说明阻塞原因（缺参数/权限/路径不可达）。\n" +
	"最终结论必须附执行证据：tool_name 与 call_id（或 task_id），以及 status/exit_code。\n" +
	"禁止承诺式空转回复（如“稍后给你结果”），除非同轮已发起工具调用。"

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
				"A2A 任务执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel；禁止使用 a2a__send/get/cancel。\n" +
				executionConsistencyPrompt,
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
				"A2A 任务执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel；禁止使用 a2a__send/get/cancel。\n" +
				executionConsistencyPrompt,
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
				"A2A 任务执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel；禁止使用 a2a__send/get/cancel。\n" +
				executionConsistencyPrompt,
		)
	default:
		return strings.TrimSpace(
			"调用 linux__bash 时仅保留 command 参数，命令内容直接写在 command 字段（不使用 cmd/timeout_sec/working_dir）；" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）；" +
				"维护写接口默认使用 JSON：/api/mcp/services/save|toggle|delete、/api/skills/save|toggle|delete|install、/api/schedules/save|toggle|delete|run；" +
				"维护写入必须使用 Content-Type: application/json；" +
				"保存定时任务时，/api/schedules/save 必填字段固定为 id,name,description,action=skill:<skill_id>,cron_expr,enabled；" +
				"A2A 接入维护统一使用 JSON 接口 /api/a2a/agents/save|toggle|delete，查询使用 /api/a2a/agents 与 /api/a2a/agents/read?id=<agent_id>；" +
				"A2A 任务执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel，禁止使用 a2a__send|get|cancel。\n" +
				executionConsistencyPrompt,
		)
	}
}
