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

#### Scenario: Large development task prefers A2A delegation
- **WHEN** 用户下达大型开发、仓库分析或多步产出类执行任务，且存在匹配的已启用 A2A agent
- **THEN** `a2a-task-orchestrator` 默认优先委托该 A2A agent 执行主体工作
- **AND** 数字分身优先承担调度、状态回读与结果汇总职责，而非先自行执行主体开发动作
