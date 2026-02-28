## ADDED Requirements
### Requirement: Metadata-Preserving Async A2A Dispatch
系统 MUST 在 `async_task__submit(task_type=a2a)` 执行链路中透传调用元数据到 A2A provider，以支持结构化执行上下文（如 `working_dir`）驱动远端 Agent 行为。

#### Scenario: Preserve caller metadata when sending A2A task
- **WHEN** 模型提交 `async_task__submit` 且 `metadata` 包含结构化字段
- **THEN** 系统将该 `metadata` 透传到 provider `Send` 请求
- **AND** 同时保留系统注入字段（如 `async_task_id`）用于链路追踪

#### Scenario: Use structured metadata instead of prompt text parsing
- **WHEN** 需要指定远端执行上下文（如工作目录）
- **THEN** 系统优先使用 `metadata` 中结构化字段进行传递
- **AND** 禁止依赖关键词/正则从用户文本中抽取并分流核心执行逻辑
