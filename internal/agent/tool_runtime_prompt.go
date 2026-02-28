package agent

import "strings"

const executionConsistencyPrompt = "" +
	"执行一致性硬约束（最高优先级）：\n" +
	"涉及执行/查询/分析本地项目/调用工具时，必须先发起工具调用；禁止先输出“我开始执行了/我先跑一下”等承诺语。\n" +
	"若本轮没有任何工具结果（function_call_output），不得声称“已执行/执行中/已完成”。\n" +
	"无法立即执行时，必须明确“我还未执行”，并说明阻塞原因（缺参数/权限/路径不可达）。\n" +
	"最终结论必须附执行证据：tool_name 与 call_id（或 task_id），以及 status/exit_code。\n" +
	"调用 async_task__submit 时，request 必须是稳定任务摘要（不含“再次/继续/重新”等轮次词）；完整执行要求放在 agent_input。\n" +
	"task_type=a2a 时，agent_input 禁止出现“调用 <agent_id>/让 <agent_id> 执行”这类调度语句，必须直接写业务目标与验收点。\n" +
	"禁止承诺式空转回复（如“稍后给你结果”），除非同轮已发起工具调用。"

func (a *Agent) buildToolRuntimePrompt() string {
	switch preferredShellName() {
	case "powershell", "pwsh":
		return strings.TrimSpace(
			"工具执行环境：linux__bash 当前在 Windows PowerShell 下执行。\n" +
				"linux__bash 仅用于本地 shell 命令（如文件/进程/时间查询），仅保留 command 参数。\n" +
				"本地 API 读取必须使用 context__read；本地 API 维护写入必须使用 maintenance__write。\n" +
				"禁止通过 linux__bash 执行任何本地 API 读写（包括 curl/Invoke-RestMethod）。\n" +
				"context__read 白名单：mcp(list)、skills(list/read)、schedules(list)、a2a(list/read)、memory(index/read/section)、async(list/get)。\n" +
				"maintenance__write 白名单：mcp(save/toggle/delete)、skills(save/toggle/delete/install)、schedules(save/toggle/delete/run)、a2a(save/toggle/delete)。\n" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。\n" +
				"保存定时任务时，/api/schedules/save 必填字段固定为：id,name,description,action=skill:<skill_id>,cron_expr,enabled（禁止使用 cron/prompt/action=reminder）。\n" +
				"A2A 任务执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel；禁止使用 a2a__send/get/cancel。\n" +
				executionConsistencyPrompt,
		)
	case "cmd":
		return strings.TrimSpace(
			"工具执行环境：linux__bash 当前在 Windows cmd 下执行。\n" +
				"linux__bash 仅用于本地 shell 命令（如文件/进程/时间查询），仅保留 command 参数。\n" +
				"写 URL 时使用正常双引号，禁止写反斜杠转义引号（如 \\\"http://...\\\"）。\n" +
				"只能使用 cmd 兼容命令（如 dir/findstr/schtasks/echo），禁止 Linux 命令（如 ls/find/head/grep/uname/crontab）。\n" +
				"本地 API 读取必须使用 context__read；本地 API 维护写入必须使用 maintenance__write。\n" +
				"禁止通过 linux__bash 执行任何本地 API 读写（包括 curl）。\n" +
				"context__read 白名单：mcp(list)、skills(list/read)、schedules(list)、a2a(list/read)、memory(index/read/section)、async(list/get)。\n" +
				"maintenance__write 白名单：mcp(save/toggle/delete)、skills(save/toggle/delete/install)、schedules(save/toggle/delete/run)、a2a(save/toggle/delete)。\n" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。\n" +
				"保存定时任务时，/api/schedules/save 必填字段固定为：id,name,description,action=skill:<skill_id>,cron_expr,enabled（禁止使用 cron/prompt/action=reminder）。\n" +
				"A2A 任务执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel；禁止使用 a2a__send/get/cancel。\n" +
				executionConsistencyPrompt,
		)
	case "bash", "sh":
		return strings.TrimSpace(
			"工具执行环境：linux__bash 当前在 Linux shell 下执行。\n" +
				"linux__bash 仅用于本地 shell 命令（如文件/进程/时间查询），仅保留 command 参数。\n" +
				"本地 API 读取必须使用 context__read；本地 API 维护写入必须使用 maintenance__write。\n" +
				"禁止通过 linux__bash 执行任何本地 API 读写（包括 curl）。\n" +
				"context__read 白名单：mcp(list)、skills(list/read)、schedules(list)、a2a(list/read)、memory(index/read/section)、async(list/get)。\n" +
				"maintenance__write 白名单：mcp(save/toggle/delete)、skills(save/toggle/delete/install)、schedules(save/toggle/delete/run)、a2a(save/toggle/delete)。\n" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。\n" +
				"保存定时任务时，/api/schedules/save 必填字段固定为：id,name,description,action=skill:<skill_id>,cron_expr,enabled（禁止使用 cron/prompt/action=reminder）。\n" +
				"A2A 任务执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel；禁止使用 a2a__send/get/cancel。\n" +
				executionConsistencyPrompt,
		)
	default:
		return strings.TrimSpace(
			"linux__bash 仅用于本地 shell 命令，禁止用于本地 API 读写；" +
				"本地 API 读取必须使用 context__read；维护写入必须使用 maintenance__write；" +
				"context__read 白名单：mcp(list)、skills(list/read)、schedules(list)、a2a(list/read)、memory(index/read/section)、async(list/get)；" +
				"maintenance__write 白名单：mcp(save/toggle/delete)、skills(save/toggle/delete/install)、schedules(save/toggle/delete/run)、a2a(save/toggle/delete)；" +
				"定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）；" +
				"/api/schedules/save 必填 id,name,description,action=skill:<skill_id>,cron_expr,enabled；" +
				"A2A 执行入口固定为 async_task__submit(task_type=a2a)，查询/取消使用 async_task__get/cancel，禁止 a2a__send|get|cancel。\n" +
				executionConsistencyPrompt,
		)
	}
}
