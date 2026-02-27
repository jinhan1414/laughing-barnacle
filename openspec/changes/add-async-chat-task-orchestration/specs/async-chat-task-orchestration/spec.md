## ADDED Requirements
### Requirement: LLM-Driven Background Task Decision
系统 MUST 允许 LLM 在对话回合中自主判断是否将任务转为后台执行，禁止由后端硬编码规则强制分流。

#### Scenario: Keep synchronous reply when model does not defer
- **WHEN** 模型判断当前请求可在前台直接完成且未调用后台任务能力
- **THEN** 系统按现有同步链路直接返回回复
- **AND** 不强制将该请求转为后台任务

#### Scenario: Defer long-running work by model decision
- **WHEN** 模型判断任务耗时较长并调用后台任务提交能力
- **THEN** 系统创建后台任务并返回 `task_id/status`
- **AND** 该结果先返回给模型作为后续回复编排输入

### Requirement: Builtin Background Task Capability
系统 MUST 以内置工具提供后台任务能力，工具名称固定为 `async_task__submit`、`async_task__get`、`async_task__cancel`，并具备显式参数校验。

#### Scenario: Submit background task successfully
- **WHEN** 模型调用 `async_task__submit` 且参数合法
- **THEN** 系统返回包含 `task_id/status` 的结果
- **AND** 后台任务状态进入 `submitted` 或 `working`

#### Scenario: Query or cancel with invalid task id
- **WHEN** 模型调用 `async_task__get` 或 `async_task__cancel` 且 `task_id` 缺失、非法或不存在
- **THEN** 系统返回显式错误
- **AND** 不得静默返回成功结果

### Requirement: Async Task Tool Parameter Contract
系统 MUST 对 `async_task__submit/get/cancel` 执行固定参数契约与强校验，禁止隐式补字段或静默容错。

#### Scenario: Validate submit required fields
- **WHEN** 模型调用 `async_task__submit` 且缺失 `task_type` 或 `request`
- **THEN** 系统返回参数错误并拒绝创建任务
- **AND** 不产生任何后台任务记录

#### Scenario: Validate submit conditional fields for a2a
- **WHEN** `async_task__submit` 的 `task_type=a2a` 且缺失 `agent_id` 或 `agent_input`
- **THEN** 系统返回参数错误并拒绝创建任务
- **AND** 错误信息明确指出缺失字段

#### Scenario: Apply get log window constraints
- **WHEN** 模型调用 `async_task__get` 并设置 `include_logs=true`
- **THEN** 系统按 `log_cursor/log_limit` 返回日志窗口
- **AND** `log_limit` 超过上限时返回显式错误

#### Scenario: Validate cancel required fields
- **WHEN** 模型调用 `async_task__cancel` 且缺失 `task_id`
- **THEN** 系统返回参数错误
- **AND** 不触发任何取消动作

### Requirement: A2A Workloads Can Be Delegated to Background Tasks
系统 MUST 允许模型将需要持续跟踪的 A2A 任务委托给 `async_task__submit`，由后台运行时在回合外完成状态跟踪与终态回传。

#### Scenario: Delegate long-running A2A task to background runtime
- **WHEN** 模型判断 A2A 任务为长时执行并调用 `async_task__submit`
- **THEN** 系统创建后台任务并记录 A2A 关联标识（至少含 `agent_id` 与远端 `task_id`）
- **AND** 后续 A2A 状态跟踪在后台执行而非同轮扩张

#### Scenario: Background A2A tracking reaches terminal state
- **WHEN** 后台运行时将 A2A 任务跟踪到终态（成功/失败/取消/超时）
- **THEN** 系统更新后台任务终态并保留可追溯执行证据
- **AND** 终态结果进入数字分身通知链路

### Requirement: Skill Strategy and Builtin Execution Separation
系统 MUST 使用内置 Skill 作为后台任务决策提示层，且实际执行固定走内置 `async_task__*` 工具，不走 Skill+HTTP 执行链路。

#### Scenario: Skill guides model to builtin tools
- **WHEN** 模型命中后台任务编排场景并读取 `async-task-orchestrator`
- **THEN** Skill 仅提供何时转后台与如何编排的策略提示
- **AND** 模型执行阶段调用 `async_task__submit/get/cancel`，而非在 Skill 中直接发 HTTP 请求

### Requirement: Fixed Background Task Index Injection
系统 MUST 在每轮上下文中固定注入后台任务索引，范围限定为“当天任务 + 仍在运行任务”，并遵循渐进式披露以控制 token 开销。

#### Scenario: Inject lightweight task index each turn
- **WHEN** 系统构建本轮模型上下文且存在后台任务记录
- **THEN** 注入轻量索引字段（至少含 `task_id/status/created_at/updated_at/brief`）
- **AND** 仅包含当天任务与仍在运行任务，默认不注入全量日志正文

#### Scenario: Read task details on demand
- **WHEN** 索引信息不足以支持当前决策
- **THEN** 模型通过 `async_task__get` 或等价只读路径按需读取单任务详情
- **AND** 未出现该条件时不拉取全量任务日志

### Requirement: Background Completion Shall Wake Up Digital Twin Notification
系统 MUST 在后台任务进入终态后唤起数字分身生成通知，并将通知主动送达用户会话。

#### Scenario: Notify user after background success
- **WHEN** 后台任务执行成功并产出结果
- **THEN** 系统唤起数字分身生成用户可读完成通知
- **AND** 通知消息写入会话并包含可追溯 `task_id`

#### Scenario: Notify user after background failure
- **WHEN** 后台任务执行失败
- **THEN** 系统唤起数字分身生成失败通知或回退为显式失败文案
- **AND** 失败原因可用于执行链路排查

### Requirement: Proactive Delivery Through Existing Chat Updates Channel
系统 MUST 通过现有 `/api/chat/updates` 增量通道返回后台任务状态与数字分身通知，不新增独立传输协议作为前置条件。

#### Scenario: Polling receives task status and notification incrementally
- **WHEN** 前端按游标轮询 `/api/chat/updates`
- **THEN** 可按时间增量看到后台任务状态事件与最终通知消息
- **AND** 用户无需再次手动提问即可获知处理结果

### Requirement: Settings Page Visibility for Background Tasks and Logs
系统 MUST 在设置页提供后台任务清单与日志可见性，支持查看全部后台任务及其全量执行日志。

#### Scenario: View full background task list in settings
- **WHEN** 用户进入设置页后台任务区域
- **THEN** 系统展示全部后台任务清单（包含进行中与历史任务）
- **AND** 清单字段至少包含 `task_id/status/created_at/updated_at`

#### Scenario: Inspect full execution logs for a task
- **WHEN** 用户查看某个后台任务详情
- **THEN** 系统展示该任务全量执行日志（状态流转、关键步骤、错误信息、通知结果）
- **AND** 日志内容与后台运行时记录一致且可追溯
