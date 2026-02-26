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

### Requirement: Autonomous A2A Agent Onboarding
系统 MUST 支持“用户提供 Agent 信息后，数字分身通过受控维护接口自主完成登记接入”，且该流程可追踪、可校验、可幂等。

#### Scenario: Register by request-based maintenance API
- **WHEN** 用户提供 `endpoint`/`agent_card_url` 并要求接入该 Agent
- **THEN** 系统调用受控维护接口完成写入 registry
- **AND** 通过 `GET /api/a2a/agents` 回读校验最终状态

#### Scenario: Reject invalid input on maintenance API
- **WHEN** 维护接口收到缺失 `endpoint` 等非法输入
- **THEN** 系统返回显式错误
- **AND** 不写入 registry

#### Scenario: Idempotent repeated registration
- **WHEN** 用户重复提供同一 Agent 信息要求接入
- **THEN** 系统更新既有记录或返回既有 `agent_id`
- **AND** 不产生重复注册记录（同 endpoint/card 签名）

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
- **THEN** `a2a-task-orchestrator` 负责选择 `agent_id` 并编排 `a2a__send/get/cancel`
- **AND** 不跨用配置维护职责

### Requirement: Progressive Disclosure for Connected A2A Agents
系统 MUST 在对话上下文中暴露已接入 A2A Agent 索引，并遵循渐进式披露原则。

#### Scenario: Inject A2A index only
- **WHEN** 本轮请求构建系统上下文且存在已接入 A2A Agent
- **THEN** 系统注入仅含索引字段（如 `agent_id/name/description/status`）的 A2A 索引
- **AND** 不注入完整 AgentCard 正文

#### Scenario: Read details on demand
- **WHEN** 索引信息不足以完成当前任务
- **THEN** 模型按需读取单个目标 Agent 的详情
- **AND** 默认单轮最多先读取 1 个 Agent 详情，不足再补充

### Requirement: Request-Based Maintenance Consistency
系统 MUST 让 A2A 接入维护沿用现有“请求式维护”链路，保持与其他系统维护能力一致。

#### Scenario: Register/update through controlled endpoints
- **WHEN** 触发 A2A 接入登记或更新
- **THEN** 系统通过 JSON 受控请求端点（`/api/a2a/agents/save|toggle|delete`）完成校验与持久化
- **AND** 返回可回读验证的结果而非仅提示词承诺

### Requirement: Execution Evidence for A2A Calls
系统 MUST 在工具执行证据中记录 A2A 关键字段，支持完整回读校验。

#### Scenario: Persist evidence for troubleshooting
- **WHEN** 任一 `a2a__*` 调用完成（成功或失败）
- **THEN** `ToolCall.Result` 中可追溯 `agent_id/task_id/status`
- **AND** 失败原因可用于链路排查（触发条件 -> 动作计划 -> 工具调用 -> 接口返回 -> 回读校验 -> 最终回复）
