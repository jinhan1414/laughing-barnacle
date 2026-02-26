## MODIFIED Requirements
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
