# a2a-native-capability Specification

## Purpose
TBD - created by archiving change add-a2a-native-capability. Update Purpose after archive.
## Requirements
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

### Requirement: In-Progress/Error Short-Circuit for A2A Tool Rounds
系统 MUST 在 A2A 任务进行中或 A2A 工具报错时直接返回确定性回复，避免同轮工具回合持续扩张导致请求超时。

#### Scenario: Return direct status when task is still running
- **WHEN** 本轮仅执行 A2A 工具且返回 `status: working/submitted`
- **THEN** 后端直接返回“任务仍在执行中（含 `task_id`）”
- **AND** 禁止继续同轮 `a2a__send/get/cancel` 轮询

#### Scenario: Return direct error on A2A tool failure
- **WHEN** 本轮仅执行 A2A 工具且出现工具错误（如 `EOF`）
- **THEN** 后端直接返回错误摘要
- **AND** 不再追加 LLM 二次收尾调用

### Requirement: Agent Routing by Registry Allowlist
系统 MUST 通过本地 A2A registry 使用 `agent_id` 路由目标 Agent，禁止模型直接指定任意 URL。

#### Scenario: Unknown agent id is rejected
- **WHEN** 模型提供未注册的 `agent_id`
- **THEN** 系统拒绝调用并返回 `agent not found or disabled` 类错误
- **AND** 不发起外部网络请求

### Requirement: Autonomous A2A Agent Onboarding
系统 MUST 支持“用户提供 Agent 信息后，数字分身通过受控维护接口自主完成登记接入”，并通过官方 SDK 执行 Agent Card 发现回填，保证接入数据可追踪、可校验、可幂等。

#### Scenario: Save agent with card discovery and normalized fields
- **WHEN** 用户通过 `/api/a2a/agents/save` 提供 `agent_card_url`
- **THEN** 系统使用官方 SDK resolver 读取 Agent Card 并回填可用字段（如 `name/description/endpoint/skills`）
- **AND** 回填结果可通过 `/api/a2a/agents*` 回读验证

#### Scenario: Reject invalid or unreachable agent card
- **WHEN** `agent_card_url` 返回非法结构或网络不可达
- **THEN** 系统返回显式错误
- **AND** 不写入不完整或脏的 A2A registry 记录

#### Scenario: Reject agent card without skills
- **WHEN** Agent Card 缺失 `skills` 字段，或可执行 Agent 的 `skills` 为空
- **THEN** 系统返回显式错误
- **AND** 不写入 registry

### Requirement: Skill Strategy Only
系统 MUST 将 Skill 限定为“策略提示层”，A2A 协议执行必须由内置工具与 provider 完成。

#### Scenario: Skill suggests agent, builtin executes
- **WHEN** Skill 命中并建议调用某个 `agent_id`
- **THEN** 模型通过 `a2a__*` 内置工具执行协议调用
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
- **AND** 索引不足时再按需读取单个 Agent 详情并编排 `a2a__send/get/cancel`

### Requirement: Progressive Disclosure for Connected A2A Agents
系统 MUST 在对话上下文中固定暴露“已启用 A2A Agent 索引”，并遵循渐进式披露原则。

#### Scenario: Inject enabled A2A index as fixed context
- **WHEN** 本轮请求构建系统上下文且存在已启用 A2A Agent
- **THEN** 系统注入仅含索引字段（如 `agent_id/name/description/status`）的 A2A 索引
- **AND** 该索引作为本轮编排默认输入，而非要求先读取 `/api/a2a/agents`

#### Scenario: Read details on demand
- **WHEN** 索引信息不足以完成当前任务
- **THEN** 模型按需读取单个目标 Agent 的详情
- **AND** 默认单轮最多先读取 1 个 Agent 详情，不足再补充

#### Scenario: Refresh list only when explicitly needed
- **WHEN** 用户明确要求获取最新接入列表或执行前需要一致性校验
- **THEN** 模型才读取 `/api/a2a/agents` 列表进行刷新
- **AND** 未出现该条件时不把列表读取作为固定首步

### Requirement: Request-Based Maintenance Consistency
系统 MUST 让 A2A 接入维护沿用现有“请求式维护”链路，保持与其他系统维护能力一致。

#### Scenario: Register/update through controlled endpoints
- **WHEN** 触发 A2A 接入登记或更新
- **THEN** 系统通过 JSON 受控请求端点（`/api/a2a/agents/save|toggle|delete`）完成校验与持久化
- **AND** 返回可回读验证的结果而非仅提示词承诺

### Requirement: Execution Evidence for A2A Calls
系统 MUST 在工具执行证据中记录 A2A 关键字段与协议字段，支持完整回读校验。

#### Scenario: Persist protocol-aware evidence for troubleshooting
- **WHEN** 任一 A2A 调用完成（成功或失败）
- **THEN** `ToolCall.Result` 中可追溯 `agent_id/task_id/status/raw_status/sdk_provider/sdk_method`
- **AND** 失败原因可用于链路排查（触发条件 -> 动作计划 -> 工具调用 -> 接口返回 -> 回读校验 -> 最终回复）

### Requirement: Official SDK Based A2A Invocation
系统 MUST 使用 A2A 官方 SDK 实现数字分身到远端 Agent 的调用链路，禁止以手写 JSON-RPC/HTTP 作为主执行实现。

#### Scenario: Use official SDK as primary invocation path
- **WHEN** 数字分身发起 A2A 调用
- **THEN** provider 通过官方 SDK 完成请求构造、发送与响应解析
- **AND** 非调试场景不走手写 JSON-RPC 直连路径

#### Scenario: Return explicit error when SDK invocation is unavailable
- **WHEN** 官方 SDK 初始化失败、版本不兼容或调用异常
- **THEN** 系统返回显式错误
- **AND** 不伪造成功结果

### Requirement: Canonical Task Status Mapping for A2A
系统 MUST 对远端 A2A 任务状态执行归一化映射，并同时保留原始状态用于诊断。

#### Scenario: Map in-progress and terminal states deterministically
- **WHEN** 远端返回 `submitted/working/input-required/auth-required/completed/failed/canceled/rejected`
- **THEN** 系统映射到本地可决策状态集合
- **AND** 原始状态写入执行证据供回读排查

#### Scenario: Expose unknown status explicitly
- **WHEN** 远端返回未识别状态
- **THEN** 系统显式返回“unknown status”类错误或告警结果
- **AND** 禁止将其静默视为成功

