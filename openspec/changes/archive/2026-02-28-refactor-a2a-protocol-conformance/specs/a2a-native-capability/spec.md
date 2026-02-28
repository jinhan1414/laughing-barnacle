## MODIFIED Requirements
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

### Requirement: Execution Evidence for A2A Calls
系统 MUST 在工具执行证据中记录 A2A 关键字段与协议字段，支持完整回读校验。

#### Scenario: Persist protocol-aware evidence for troubleshooting
- **WHEN** 任一 A2A 调用完成（成功或失败）
- **THEN** `ToolCall.Result` 中可追溯 `agent_id/task_id/status/raw_status/sdk_provider/sdk_method`
- **AND** 失败原因可用于链路排查（触发条件 -> 动作计划 -> 工具调用 -> 接口返回 -> 回读校验 -> 最终回复）

## ADDED Requirements
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
