# Change: 增加 LLM 自主判定的后台任务能力并由数字分身主动通知结果

## Why
当前方案是由后端统一决定同步/异步，模型没有主导权。  
这与“执行链路优化优先依赖模型能力”的长期规则不一致，也限制了数字分身按任务复杂度自主编排的能力。  
系统已经具备主动消息基础（后台可写会话、前端可增量拉取），可作为后台执行结果通知通道。

## What Changes
- 新增内置 Skill（策略层）指导 LLM 何时转后台：
  - 新增 `async-task-orchestrator`，仅负责决策提示与调用编排。
  - Skill 不直接执行 HTTP，请求执行固定走内置工具。
- 新增“数字分身后台任务能力”（内置工具/运行时）供 LLM 主动调用：
  - LLM 根据任务复杂度自主判断是否转后台，不由后端硬编码分流。
  - 工具名称与职责固定为：
    - `async_task__submit`：提交后台任务，返回 `task_id/status`
    - `async_task__get`：查询任务状态/结果/日志
    - `async_task__cancel`：取消可取消任务
  - 参数契约固定（结构化、强校验）：
    - `async_task__submit`：`task_type`、`request` 必填；当 `task_type=a2a` 时 `agent_id`、`agent_input` 条件必填
    - `async_task__get`：`task_id` 必填；`include_logs`、`log_cursor`、`log_limit` 可选
    - `async_task__cancel`：`task_id` 必填；`reason` 可选
  - 所有工具结果先返回给模型，再由模型组织面向用户的回复。
  - 后台任务类型支持 A2A 场景（如远端 agent 长任务），用于承接 A2A 的后续状态跟踪与完成通知。
- 保持现有聊天入口默认同步：
  - `/chat/send` 不强制改为全量异步。
  - 当 LLM 判定需要后台执行时，在同轮通过工具提交后台任务，由模型自行决定当轮回复文案。
- 增加后台任务索引固定注入：
  - 类似 Skill/A2A 索引，每轮固定注入“当天后台任务摘要索引”（附带仍在运行任务）。
  - 详情按需通过 `async_task__get` 或本地只读接口读取，避免一次性注入全量日志。
- 增加“结果唤起通知”链路：
  - 后台任务完成或失败后，唤起数字分身生成用户可读通知并写入会话。
  - 通知通过现有 `/api/chat/updates` 增量下发到聊天页。
- A2A 与后台任务一体化整改：
  - **BREAKING**：移除模型侧直接调用 `a2a__send/get/cancel` 的执行入口，不做兼容保留。
  - A2A 统一通过 `async_task__submit(type=a2a)` 提交，由后台 worker 负责远端任务创建、跟踪与终态通知。
  - 结果查询统一走 `async_task__get`（含关联 `agent_id`、远端 `task_id`、执行日志）。
- 设置页新增后台任务可观测面板：
  - 可查看后台任务清单（含历史任务）。
  - 可查看每个任务的全量执行日志与状态流转记录。
- 明确失败暴露要求：
  - 后台任务与唤起通知任一阶段失败都要显式记录错误证据，禁止静默降级。

## Impact
- Affected specs: `async-chat-task-orchestration`, `a2a-native-capability`
- Affected code:
  - `internal/agent/reply_generation.go`
  - `internal/agent/tool_call.go`
  - `internal/agent/a2a_round_control.go`
  - `internal/agent/a2a_tools.go`
  - `internal/agent/tool_runtime_prompt.go`
  - `internal/agent/turn_handlers.go`
  - `data/skills/async-task-orchestrator/SKILL.md`
  - `internal/web/server_api_chat.go`
  - `internal/web/server_pages.go`
  - `internal/web/server_setup.go`
  - `internal/conversation/store_messages.go`
  - `cmd/server/main.go`
  - `internal/web/templates/chat.html`
  - `internal/web/templates/settings.html`
  - `internal/web/*test.go`、`internal/agent/*test.go`
