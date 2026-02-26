package agent

import "strings"

func (a *Agent) buildToolRuntimePrompt() string {
	switch preferredShellName() {
	case "powershell", "pwsh":
		return strings.TrimSpace(
			"工具执行环境：linux__bash 当前在 Windows PowerShell 下执行。\n" +
				"调用参数键必须是 command（不是 cmd）。\n" +
				"若需调用 curl，请使用 curl.exe（避免命中 PowerShell 的 curl 别名）。\n" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。\n" +
				"创建/更新 Skill 固定使用 POST /settings/skills/save（禁止 /api/skills/save）。\n" +
				"保存定时任务时，/settings/schedules/save 必填字段固定为：id,name,description,action=skill:<skill_id>,cron_expr,enabled=on（禁止使用 cron/prompt/action=reminder）。\n" +
				"A2A 接入维护统一使用 JSON 接口：POST /api/a2a/agents/save、/api/a2a/agents/toggle、/api/a2a/agents/delete；查询使用 GET /api/a2a/agents（详情 /api/a2a/agents/read?id=<agent_id>）。\n" +
				"A2A JSON 写入优先使用 Invoke-RestMethod + ConvertTo-Json，避免在命令行手写多层转义。",
		)
	case "cmd":
		return strings.TrimSpace(
			"工具执行环境：linux__bash 当前在 Windows cmd 下执行。\n" +
				"调用参数键必须是 command（不是 cmd）。\n" +
				"写 URL 时使用正常双引号，禁止写反斜杠转义引号（如 \\\"http://...\\\"）。\n" +
				"只能使用 cmd 兼容命令（如 curl、dir、findstr、schtasks、echo），禁止使用 Linux 命令（如 ls/find/head/grep/uname/crontab）。\n" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。\n" +
				"创建/更新 Skill 固定使用 POST /settings/skills/save（禁止 /api/skills/save）。\n" +
				"使用 --data-urlencode 时，每个字段都必须写成 --data-urlencode \"key=value\"（禁止省略双引号）。\n" +
				"在 cmd + /settings/*/save 场景，优先使用单条 -d \"k=v&k2=v2\"（值先 URL 编码），避免 -F 与多段参数拆分。\n" +
				"保存定时任务时，/settings/schedules/save 必填字段固定为：id,name,description,action=skill:<skill_id>,cron_expr,enabled=on（禁止使用 cron/prompt/action=reminder）。\n" +
				"A2A 接入维护统一使用 JSON 接口：POST /api/a2a/agents/save、/api/a2a/agents/toggle、/api/a2a/agents/delete；查询使用 GET /api/a2a/agents（详情 /api/a2a/agents/read?id=<agent_id>）。\n" +
				"A2A JSON 写入必须带 -H \"Content-Type: application/json\"，并使用单条 -d \"{\\\"key\\\":...}\"。",
		)
	case "bash", "sh":
		return strings.TrimSpace(
			"工具执行环境：linux__bash 当前在 Linux shell 下执行。\n" +
				"调用参数键必须是 command（不是 cmd）。\n" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。\n" +
				"创建/更新 Skill 固定使用 POST /settings/skills/save（禁止 /api/skills/save）。\n" +
				"保存定时任务时，/settings/schedules/save 必填字段固定为：id,name,description,action=skill:<skill_id>,cron_expr,enabled=on（禁止使用 cron/prompt/action=reminder）。\n" +
				"A2A 接入维护统一使用 JSON 接口：POST /api/a2a/agents/save、/api/a2a/agents/toggle、/api/a2a/agents/delete；查询使用 GET /api/a2a/agents（详情 /api/a2a/agents/read?id=<agent_id>）。",
		)
	default:
		return strings.TrimSpace(
			"调用 linux__bash 时，参数键必须是 command（不是 cmd）；" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）；" +
				"创建/更新 Skill 固定使用 POST /settings/skills/save（禁止 /api/skills/save）；" +
				"保存定时任务时，/settings/schedules/save 必填字段固定为 id,name,description,action=skill:<skill_id>,cron_expr,enabled=on；" +
				"A2A 接入维护统一使用 JSON 接口 /api/a2a/agents/save|toggle|delete，查询使用 /api/a2a/agents 与 /api/a2a/agents/read?id=<agent_id>。",
		)
	}
}
