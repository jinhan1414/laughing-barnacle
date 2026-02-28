## ADDED Requirements
### Requirement: Official SDK Implementation for codex-a2a
`integrations/codex-a2a` MUST 使用 A2A 官方 Python SDK（`a2a-python`/`a2a-sdk`）实现服务端协议能力，禁止手写协议栈作为主实现。

#### Scenario: Start service with official Python SDK runtime
- **WHEN** 启动 `integrations/codex-a2a` 服务
- **THEN** 服务通过官方 Python SDK 暴露 A2A 能力
- **AND** 未安装或版本不兼容时返回显式启动错误

### Requirement: Standard Agent Card Exposure for codex-a2a
`integrations/codex-a2a` MUST 在 `/.well-known/agent-card.json` 暴露可互操作的 Agent Card，至少包含协议版本、身份描述、调用端点与 `skills`。

#### Scenario: Return complete agent card fields
- **WHEN** 客户端请求 `/.well-known/agent-card.json`
- **THEN** 服务返回结构化 Agent Card（含 `name/description/url`、协议版本字段与 `skills`）
- **AND** 字段可被数字分身接入流程直接解析使用

#### Scenario: Expose at least one executable skill for codex task
- **WHEN** `codex-a2a` 作为可执行远端 Agent 暴露 Agent Card
- **THEN** `skills` 至少包含 1 个可执行能力声明
- **AND** 该 skill 可映射到实际 codex 任务执行路径

### Requirement: JSON-RPC Task Lifecycle Contract for codex-a2a
`integrations/codex-a2a` MUST 基于官方 SDK 提供稳定的任务生命周期契约（`send/get/cancel`），并维持可观测任务状态机。

#### Scenario: Submit task and return accepted state
- **WHEN** 客户端调用任务发送方法并参数合法
- **THEN** 服务立即返回 `task_id` 与进行中状态（如 `submitted/working`）
- **AND** 后台异步执行实际 Codex 任务

#### Scenario: Query task and return terminal artifacts
- **WHEN** 客户端查询已完成任务
- **THEN** 服务返回终态状态与产物内容
- **AND** 若任务失败，返回显式错误消息

#### Scenario: Cancel running task deterministically
- **WHEN** 客户端取消进行中任务
- **THEN** 服务尝试终止对应进程并将任务标记为 `canceled`
- **AND** 后续查询结果与取消状态一致

### Requirement: Explicit Error Semantics for codex-a2a
`integrations/codex-a2a` MUST 对非法输入、未知方法和执行失败返回显式错误，禁止静默降级或 mock 成功。

#### Scenario: Reject unsupported method
- **WHEN** 客户端调用未支持的 JSON-RPC 方法
- **THEN** 服务返回标准错误对象
- **AND** 不返回伪造成功结果

#### Scenario: Surface codex execution failure
- **WHEN** Codex CLI 执行失败或输出不可用
- **THEN** 服务将失败原因写入任务结果并返回失败状态
- **AND** 调用方可据此执行重试或人工排查
