package agent

import "strings"

const executionConsistencyPrompt = "" +
	"执行一致性硬约束（最高优先级）：\n" +
	"涉及执行/查询/分析本地项目/调用工具时，先发起工具调用，再基于工具结果回复。\n" +
	"若本轮没有工具结果（function_call_output），明确回复“我还未执行”，并说明阻塞原因（缺参数/权限/路径不可达）。\n" +
	"最终结论必须附执行证据：tool_name 与 call_id（或 task_id），以及 status/exit_code。\n" +
	"大型开发、仓库分析、多步产出类任务：若已有匹配的已启用 A2A agent，优先委托该 agent 执行主体工作；数字分身负责调度、回读与汇总，不要先展开本地主体开发。\n" +
	"调用 async_task__submit 时，request 必须是稳定任务摘要（不含“再次/继续/重新”等轮次词）；完整执行要求放在 agent_input。\n" +
	"task_type=a2a 时，agent_input 直接写业务目标与验收点，不写“调用 <agent_id>/让 <agent_id> 执行”这类调度语句。\n" +
	"若任务是多步且需要在后台结果返回后自动继续，必须在提交 async_task 后调用 autonomous_run__checkpoint 落盘 run 状态。\n" +
	"恢复 autonomous run 时，结束前必须再次调用 autonomous_run__checkpoint，禁止只用自然语言描述“我会继续”。\n" +
	"在获得工具返回结果前，保持简洁说明并等待执行证据，不输出承诺式进度话术。"

func (a *Agent) buildToolRuntimePrompt() string {
	switch preferredShellName() {
	case "powershell", "pwsh":
		return strings.TrimSpace(
			"# Tool & Environment Constraints (工具与环境硬约束)\n" +
				"工具执行环境：bash 当前在 Windows PowerShell 下执行。\n" +
				"bash 仅用于本地 shell 命令（如文件/进程/时间查询），仅保留 command 参数。\n" +
				"涉及本地 API 读取时优先调用 context__read；涉及维护写入时优先调用 maintenance__write。\n" +
				"skill 文档与 references 优先用 context__read(resource=\"skills\", action=\"index|read\")；skill 脚本仅在确有必要时使用 bash 执行。\n" +
				"本地 API 的路由白名单、字段必填与格式校验由工具 schema 与服务端统一执行。\n" +
				"定时任务列表统一使用 context__read(resource=\"schedules\", action=\"list\")。\n" +
				"A2A 任务执行统一使用 async_task__submit(task_type=a2a)，查询与取消统一使用 async_task__get/cancel。\n" +
				executionConsistencyPrompt,
		)
	case "cmd":
		return strings.TrimSpace(
			"# Tool & Environment Constraints (工具与环境硬约束)\n" +
				"工具执行环境：bash 当前在 Windows cmd 下执行。\n" +
				"bash 仅用于本地 shell 命令（如文件/进程/时间查询），仅保留 command 参数。\n" +
				"写 URL 时使用正常双引号，例如 curl -sS \"http://127.0.0.1:9080/...\"。\n" +
				"优先使用 cmd 兼容命令（如 dir/findstr/schtasks/echo）。\n" +
				"涉及本地 API 读取时优先调用 context__read；涉及维护写入时优先调用 maintenance__write。\n" +
				"skill 文档与 references 优先用 context__read(resource=\"skills\", action=\"index|read\")；skill 脚本仅在确有必要时使用 bash 执行。\n" +
				"本地 API 的路由白名单、字段必填与格式校验由工具 schema 与服务端统一执行。\n" +
				"定时任务列表统一使用 context__read(resource=\"schedules\", action=\"list\")。\n" +
				"A2A 任务执行统一使用 async_task__submit(task_type=a2a)，查询与取消统一使用 async_task__get/cancel。\n" +
				executionConsistencyPrompt,
		)
	case "bash", "sh":
		return strings.TrimSpace(
			"# Tool & Environment Constraints (工具与环境硬约束)\n" +
				"工具执行环境：bash 当前在 Linux shell 下执行。\n" +
				"bash 仅用于本地 shell 命令（如文件/进程/时间查询），仅保留 command 参数。\n" +
				"涉及本地 API 读取时优先调用 context__read；涉及维护写入时优先调用 maintenance__write。\n" +
				"skill 文档与 references 优先用 context__read(resource=\"skills\", action=\"index|read\")；skill 脚本仅在确有必要时使用 bash 执行。\n" +
				"本地 API 的路由白名单、字段必填与格式校验由工具 schema 与服务端统一执行。\n" +
				"定时任务列表统一使用 context__read(resource=\"schedules\", action=\"list\")。\n" +
				"A2A 任务执行统一使用 async_task__submit(task_type=a2a)，查询与取消统一使用 async_task__get/cancel。\n" +
				executionConsistencyPrompt,
		)
	default:
		return strings.TrimSpace(
			"# Tool & Environment Constraints (工具与环境硬约束)\n" +
				"bash 用于本地 shell 命令；本地 API 读取优先使用 context__read，维护写入优先使用 maintenance__write；" +
				"skill 文档与 references 优先用 context__read(resource=\"skills\", action=\"index|read\")，skill 脚本仅在确有必要时使用 bash 执行；" +
				"本地 API 的路由白名单、字段必填与格式校验由工具 schema 与服务端统一执行；" +
				"定时任务列表统一使用 context__read(resource=\"schedules\", action=\"list\")；" +
				"A2A 执行统一使用 async_task__submit(task_type=a2a)，查询与取消统一使用 async_task__get/cancel。\n" +
				executionConsistencyPrompt,
		)
	}
}
