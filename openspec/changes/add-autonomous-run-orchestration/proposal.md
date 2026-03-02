# Change: 增加事件驱动的自主运行状态机

## Why
当前系统已经具备 `async_task` 与 A2A 长任务跟踪能力，但后台任务终态后只会追加一条通知消息，无法把“多步目标”自动推进到下一步。  
这导致数字分身仍停留在“会异步执行单步任务”，而不是“可无人值守持续完成多步任务”。

## What Changes
- 新增 `autonomous_run` 运行时，用于持久化多步目标的状态、等待条件、步骤轨迹与结构化上下文。
- 新增内置工具 `autonomous_run__checkpoint`，由 LLM 显式提交 run checkpoint，禁止靠自然语言隐式推断 run 状态。
- 新增 `resume_run` 链路：
  - 当 `async_task` 进入终态且命中等待中的 run 时，系统自动恢复执行该 run。
  - 恢复执行时为 LLM 构造专用上下文包（固定前缀 + run 快照 + 最新事件 + 最近步骤摘要 + 可执行索引）。
- 在聊天页增加 `autonomous_run_status` 事件展示与运行中状态摘要。
- 在设置页增加“自主运行”可视化区域，支持查看 run 列表、等待条件、错误信息与步骤轨迹。
- 允许通过 `context__read(resource="runs", action="list|get")` 按需读取 run 索引与详情。
- 保持失败显式暴露：
  - run 恢复时若模型未写 checkpoint，系统显式标记 run 失败并记录证据。
  - 禁止静默吞掉恢复错误或伪造继续执行成功。

## Impact
- Affected specs:
  - `autonomous-run-orchestration`
- Affected code:
  - `internal/agent/agent.go`
  - `internal/agent/reply_generation.go`
  - `internal/agent/tool_call.go`
  - `internal/agent/context_read_tool.go`
  - `internal/agent/tool_runtime_prompt.go`
  - `internal/agent/async_task_notifications.go`
  - `internal/conversation/store*.go`
  - `internal/web/server.go`
  - `internal/web/server_pages.go`
  - `internal/web/server_api_chat_realtime.go`
  - `internal/web/server_chat.go`
  - `internal/web/templates/chat.html`
  - `internal/web/templates/settings.html`
  - `builtin-skills/autonomous-run-orchestrator/SKILL.md`
  - `internal/agent/*test.go`
  - `internal/web/*test.go`
  - `internal/skills/*test.go`
