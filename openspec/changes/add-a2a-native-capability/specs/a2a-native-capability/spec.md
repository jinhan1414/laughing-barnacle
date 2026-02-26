## ADDED Requirements
### Requirement: Native A2A Provider Injection
系统 MUST 支持在 Agent 层注入原生 `A2AProvider`，其生命周期与 `MemoryProvider` 同级管理。

#### Scenario: Agent starts with A2A provider
- **WHEN** 服务启动并为 Agent 设置 A2A provider
- **THEN** Agent 可在工具回合中调用 A2A provider
- **AND** 不影响现有 memory/skill provider 行为

#### Scenario: Agent runs without A2A provider
- **WHEN** 未配置 A2A provider 且模型调用 A2A 内置工具
- **THEN** 系统返回显式错误
- **AND** 禁止静默降级为 Skill 脚本或 mock 结果

### Requirement: Builtin A2A Tool Execution
系统 MUST 提供内置工具 `a2a__send`、`a2a__get`、`a2a__cancel`，并由 `callBuiltinTool` 直接分发执行。

#### Scenario: Send task to remote agent
- **WHEN** 模型调用 `a2a__send` 且参数合法
- **THEN** 系统向目标 Agent 发起任务
- **AND** 返回至少包含 `agent_id`、`task_id`、`status` 的结果

#### Scenario: Query and cancel task
- **WHEN** 模型调用 `a2a__get` 或 `a2a__cancel`
- **THEN** 系统返回对应任务状态或取消结果
- **AND** 任务不存在或状态非法时返回显式错误

### Requirement: Agent Routing by Registry Allowlist
系统 MUST 通过本地 A2A registry 使用 `agent_id` 路由目标 Agent，禁止模型直接指定任意 URL。

#### Scenario: Unknown agent id is rejected
- **WHEN** 模型提供未注册的 `agent_id`
- **THEN** 系统拒绝调用并返回 `agent not found or disabled` 类错误
- **AND** 不发起外部网络请求

### Requirement: Skill Strategy Only
系统 MUST 将 Skill 限定为“策略提示层”，A2A 协议执行必须由内置工具与 provider 完成。

#### Scenario: Skill suggests agent, builtin executes
- **WHEN** Skill 命中并建议调用某个 `agent_id`
- **THEN** 模型通过 `a2a__*` 内置工具执行协议调用
- **AND** Skill 内容不包含直接执行 A2A 请求的成功承诺

### Requirement: Execution Evidence for A2A Calls
系统 MUST 在工具执行证据中记录 A2A 关键字段，支持完整回读校验。

#### Scenario: Persist evidence for troubleshooting
- **WHEN** 任一 `a2a__*` 调用完成（成功或失败）
- **THEN** `ToolCall.Result` 中可追溯 `agent_id/task_id/status`
- **AND** 失败原因可用于链路排查（触发条件 -> 动作计划 -> 工具调用 -> 接口返回 -> 回读校验 -> 最终回复）
