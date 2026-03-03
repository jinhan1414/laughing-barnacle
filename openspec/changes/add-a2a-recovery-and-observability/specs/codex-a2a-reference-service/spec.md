## ADDED Requirements
### Requirement: Persistent Task Store for codex-a2a
`integrations/codex-a2a` SHALL 持久化任务状态与产物索引，避免服务重启后全部任务状态丢失。

#### Scenario: Query task after service restart
- **WHEN** `codex-a2a` 服务重启后收到 `tasks/get`
- **THEN** 服务仍可从本地持久化 store 读取历史任务
- **AND** 返回与重启前一致的已持久化状态与产物索引

### Requirement: Explicit Orphan Handling on Restart
`integrations/codex-a2a` SHALL 在服务启动时显式处理重启前遗留的非终态任务。

#### Scenario: Mark orphaned working task as failed on boot
- **WHEN** 持久化 store 中存在 `submitted/working` 等非终态任务，但对应执行进程已无法恢复
- **THEN** 服务启动后将该任务标记为显式失败
- **AND** 失败原因包含 `service_restarted_orphaned_task` 类证据

### Requirement: Health and Debug Endpoints for codex-a2a
`integrations/codex-a2a` SHALL 提供健康检查与任务调试接口，支持长任务观测。

#### Scenario: Health endpoint reports runtime state
- **WHEN** 客户端请求 `/healthz`
- **THEN** 服务返回健康状态、启动时间、工作目录、输出目录与活跃任务数
- **AND** 结果可用于判断服务是否正常运行

#### Scenario: Debug task endpoints expose progress detail
- **WHEN** 客户端请求 `/debug/tasks` 或 `/debug/tasks/{task_id}`
- **THEN** 服务返回任务摘要或单任务详情
- **AND** 详情至少包含当前状态、最近更新时间、阶段信息或证据文件路径
