## MODIFIED Requirements
### Requirement: JSON-RPC Task Lifecycle Contract for codex-a2a
`integrations/codex-a2a` MUST 基于官方 SDK 提供稳定的任务生命周期契约（`send/get/cancel`），并以 Codex 回合终态证据判定任务完成，而不是仅以进程退出码或最后一句文本判定完成。

#### Scenario: Submit task and return accepted state
- **WHEN** 客户端调用任务发送方法并参数合法
- **THEN** 服务立即返回 `task_id` 与进行中状态（如 `submitted/working`）
- **AND** 后台异步执行实际 Codex 任务

#### Scenario: Complete only when turn completion evidence exists
- **WHEN** 客户端查询任务且 Codex 执行已产生完整回合终态证据（至少含 `turn.completed` 与最终 `agent_message`）
- **THEN** 服务返回终态 `completed` 与最终产物
- **AND** 终态产物可回读到最终答复与关键执行证据

#### Scenario: Fail explicitly when terminal evidence is missing
- **WHEN** Codex 进程已退出但缺失回合终态证据（如无 `turn.completed` 或无最终 `agent_message`）
- **THEN** 服务返回显式失败状态与错误摘要
- **AND** 禁止将该任务静默标记为 `completed`

#### Scenario: Cancel running task deterministically
- **WHEN** 客户端取消进行中任务
- **THEN** 服务尝试终止对应进程并将任务标记为 `canceled`
- **AND** 后续查询结果与取消状态一致

## ADDED Requirements
### Requirement: Full Filesystem Execution Mode for codex-a2a
`integrations/codex-a2a` MUST 以可访问完整主机磁盘的模式执行 `codex exec`（等价于 `approval=never` 且 `sandbox=danger-full-access` 的配置）。

#### Scenario: Run codex exec with full-access execution flags
- **WHEN** `codex-a2a` 启动任务执行
- **THEN** 传递全盘访问执行参数给 `codex exec`
- **AND** 不因默认沙盒限制阻断跨目录任务

### Requirement: Request-Scoped Working Directory for codex-a2a
`integrations/codex-a2a` MUST 支持从请求 `metadata` 读取 `working_dir` 作为单任务工作目录，并进行显式校验。

#### Scenario: Use metadata working directory when provided
- **WHEN** A2A 请求 `metadata.working_dir` 存在且目录可访问
- **THEN** 服务使用该目录作为本次 `codex exec -C` 工作目录
- **AND** 执行证据可回读本次生效目录

#### Scenario: Return explicit error for invalid working directory
- **WHEN** `metadata.working_dir` 不存在、不可访问或非法
- **THEN** 服务返回显式错误并标记任务失败
- **AND** 不静默回退到其他目录

### Requirement: Contract-Agnostic Output for codex-a2a
`integrations/codex-a2a` MUST 保持通用任务输出能力，不得强制特定业务 schema 或关键词作为完成前提。

#### Scenario: Accept generic final response without domain schema
- **WHEN** Codex 返回任意任务类型的有效最终答复
- **THEN** 服务将其作为终态产物返回
- **AND** 不要求命中特定字段模板（如固定分析报告结构）

### Requirement: Minimal Runtime Prompt Prefix for codex-a2a
`integrations/codex-a2a` MUST 在每次 `codex exec` 调用时注入稳定且最小的默认提示词前缀，用于约束执行收敛行为，而非约束业务输出结构。

#### Scenario: Prefix enforces execution convergence
- **WHEN** 服务构造 `codex exec` 输入
- **THEN** 默认前缀明确要求“不得只输出计划即结束、需继续执行直至可交付结果、失败显式返回原因与证据”
- **AND** 该前缀作为稳定输入前缀被一致注入

#### Scenario: Prefix does not impose output schema
- **WHEN** 服务注入默认前缀
- **THEN** 前缀不要求固定业务字段、固定 JSON 模板或关键词命中
- **AND** 不影响通用任务类型的自然输出
