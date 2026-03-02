## ADDED Requirements
### Requirement: Structured Autonomous Run Checkpoint
系统 MUST 通过结构化 checkpoint 持久化自主运行状态，禁止仅依赖自然语言推断 run 当前所处步骤与等待条件。

#### Scenario: Create autonomous run by checkpoint
- **WHEN** 模型调用 `autonomous_run__checkpoint` 且未提供 `run_id`
- **THEN** 系统创建新的自主运行记录
- **AND** 返回包含 `run_id/status/current_step` 的结果

#### Scenario: Update autonomous run by checkpoint
- **WHEN** 模型调用 `autonomous_run__checkpoint` 且提供已存在的 `run_id`
- **THEN** 系统更新该 run 的状态、等待条件、上下文补丁与步骤轨迹
- **AND** 不得创建重复 run

### Requirement: Checkpoint Parameter Contract
系统 MUST 对 `autonomous_run__checkpoint` 执行固定参数契约与强校验，禁止静默容错。

#### Scenario: Reject waiting_async without async reference
- **WHEN** checkpoint 的 `status=waiting_async` 但缺失 `waiting_ref`
- **THEN** 系统返回显式参数错误
- **AND** 不更新 run 状态

#### Scenario: Reject create without goal
- **WHEN** 模型创建新 run 但 `goal` 为空
- **THEN** 系统返回显式参数错误
- **AND** 不创建 run 记录

### Requirement: Resume Autonomous Run on Async Task Terminal Event
系统 MUST 在等待异步任务的自主运行命中终态事件时自动恢复执行，而不是等待用户再次发言。

#### Scenario: Resume run when async task completes
- **WHEN** `async_task` 进入终态且存在 `waiting_async`、`waiting_ref=task_id` 的 run
- **THEN** 系统自动触发 `resume_run`
- **AND** 恢复执行时将该终态事件摘要提供给模型

#### Scenario: Fail run when resume round does not checkpoint
- **WHEN** `resume_run` 完成后模型未写入新的 autonomous run checkpoint
- **THEN** 系统显式将该 run 标记为 `failed`
- **AND** 失败原因包含“missing checkpoint”类证据

### Requirement: Structured Resume Context Package
系统 MUST 在 `resume_run` 时为模型提供结构化上下文包，而不是依赖全量聊天历史回放。

#### Scenario: Resume context includes run snapshot and latest event
- **WHEN** 系统恢复某个自主运行
- **THEN** 注入至少包含 `goal/current_step/status/waiting_type/waiting_ref` 的 run 快照
- **AND** 同时注入触发恢复的最新事件摘要

#### Scenario: Resume context uses progressive disclosure
- **WHEN** 系统构建 resume 上下文
- **THEN** 默认只注入最近步骤摘要与可执行索引
- **AND** 不默认注入全量日志或全文 artifact

### Requirement: Autonomous Run Visibility in Chat and Settings
系统 MUST 让用户在聊天页直接感知自主运行状态，并在设置页查看 run 列表与步骤轨迹。

#### Scenario: Chat updates include autonomous run status
- **WHEN** 自主运行状态发生变化
- **THEN** 系统通过现有聊天增量通道返回 `autonomous_run_status` 事件
- **AND** 聊天页可展示当前步骤与状态摘要

#### Scenario: Settings page shows runs and steps
- **WHEN** 用户进入设置页自主运行区域
- **THEN** 系统展示 run 列表、等待条件、错误信息与步骤轨迹
- **AND** 展示内容与实际持久化状态一致

### Requirement: Context Read Access for Autonomous Runs
系统 MUST 支持通过受控只读路径按需读取 run 索引与详情。

#### Scenario: Read autonomous run list and details on demand
- **WHEN** 模型或系统调用 `context__read(resource="runs", action="list|get")`
- **THEN** 系统返回 run 索引或指定 run 详情
- **AND** 非法 `run_id` 返回显式错误
