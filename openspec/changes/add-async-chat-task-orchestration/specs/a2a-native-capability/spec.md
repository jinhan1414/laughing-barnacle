## REMOVED Requirements
### Requirement: Builtin A2A Tool Execution
**Reason**: A2A 执行入口统一收敛到 `async_task__submit(type=a2a)`，不再向模型暴露 `a2a__send/get/cancel`。  
**Migration**: 模型与技能中的 A2A 发起、查询、取消调用统一迁移为 `async_task__submit/get/cancel`。

### Requirement: In-Progress/Error Short-Circuit for A2A Tool Rounds
**Reason**: 同轮 A2A 直连工具入口被移除，不再存在 A2A 工具回合同轮扩张场景。  
**Migration**: A2A 长任务状态管理迁移到后台任务运行时，由后台跟踪终态并主动通知。

## MODIFIED Requirements
### Requirement: Skill Strategy Only
系统 MUST 将 Skill 限定为“策略提示层”，A2A 协议执行必须由内置后台任务工具完成。

#### Scenario: Skill suggests async execution, builtin executes
- **WHEN** Skill 命中并建议调用某个 `agent_id`
- **THEN** 模型通过 `async_task__submit(type=a2a)` 发起执行
- **AND** Skill 内容不包含直接执行 A2A 请求的成功承诺

### Requirement: Two Dedicated A2A Skills
系统 MUST 提供两个 A2A 专用 Skill：`a2a-config-maintainer` 与 `a2a-task-orchestrator`，并保持职责分离。

#### Scenario: Config maintainer handles onboarding intent
- **WHEN** 用户要求添加或维护 A2A 接入
- **THEN** `a2a-config-maintainer` 负责触发登记/维护编排
- **AND** 实际写操作通过受控请求接口完成（JSON body）

#### Scenario: Task orchestrator handles execution intent
- **WHEN** 用户要求调用外部 Agent 完成任务
- **THEN** `a2a-task-orchestrator` 先基于本轮已注入的 A2A 索引选择 `agent_id`
- **AND** 通过 `async_task__submit(type=a2a)` 发起任务并在必要时调用 `async_task__get/cancel`

### Requirement: Execution Evidence for A2A Calls
系统 MUST 在执行证据中记录 A2A 与后台任务关键字段，支持完整回读校验。

#### Scenario: Persist evidence for troubleshooting
- **WHEN** 任一 A2A 相关后台任务完成（成功或失败）
- **THEN** 执行证据中可追溯 `agent_id/remote_task_id/async_task_id/status`
- **AND** 失败原因可用于链路排查（触发条件 -> 动作计划 -> 工具调用 -> 接口返回 -> 回读校验 -> 最终回复）

## ADDED Requirements
### Requirement: A2A Execution via Async Task Gateway
系统 MUST 通过 `async_task__submit(type=a2a)` 作为模型发起 A2A 执行的唯一入口。

#### Scenario: Create A2A task through async gateway
- **WHEN** 模型请求执行 A2A 任务
- **THEN** 系统仅接受 `async_task__submit(type=a2a)` 作为发起入口
- **AND** 返回后台任务标识与受理状态

#### Scenario: Enforce A2A submit parameter contract
- **WHEN** 模型调用 `async_task__submit(type=a2a)` 但缺失 `agent_id` 或 `agent_input`
- **THEN** 系统返回参数错误并拒绝创建任务
- **AND** 不发起任何远端 A2A 请求

#### Scenario: Query or cancel A2A task through async gateway
- **WHEN** 模型需要查询或取消 A2A 任务
- **THEN** 系统通过 `async_task__get/cancel` 处理
- **AND** 不再要求模型直接调用 `a2a__get/cancel`
