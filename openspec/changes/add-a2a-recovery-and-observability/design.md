## Context
当前 A2A 长任务问题同时发生在三层：

1. 主服务查询链路每次 `GetTask/CancelTask` 都会重新依赖 Agent Card 发现，短暂网络故障会把“状态查询”放大成“整体不可达”。
2. Async task tracker 在连续错误后会暂停，但缺少独立的自动恢复调度。
3. `codex-a2a` 使用 `InMemoryTaskStore`，服务重启后任务状态与阶段观测全丢，且没有健康/调试接口。

## Goals / Non-Goals
- Goals:
  - 让 A2A 长任务在短暂服务抖动后可自动恢复跟踪
  - 让 `codex-a2a` 在长时间执行时可被健康检查与任务调试接口观测
  - 服务重启后显式处理 orphaned 任务，避免无限 `working`
- Non-Goals:
  - 不在本次提案中实现远端 subprocess 跨进程真正续跑
  - 不重做整个 A2A registry 或 async task UI 结构

## Decisions
### Decision 1: Query path uses endpoint-first strategy
`Send` 可继续依赖 Agent Card/endpoint 混合能力；但 `GetTask/CancelTask` 对已经登记过的 Agent 优先使用持久化 endpoint 建 client，不再把查询阶段绑定到 Agent Card 可达性。

### Decision 2: Tracker pause schedules bounded auto-recovery
当 tracker 因连续错误或 reconcile 错误进入 `paused` 时，系统写入下一次恢复时间，并异步调度一次恢复 worker。若恢复后仍失败，可再次进入 pause + 下次恢复，保持显式日志。

### Decision 3: codex-a2a persists tasks to local JSON store
`codex-a2a` 引入基于文件的 `TaskStore` 实现，保证任务状态、历史、artifact 在服务重启后仍可查询。

### Decision 4: Restarted service marks orphaned in-progress tasks explicitly
由于 subprocess 无法跨进程恢复，`codex-a2a` 启动时将持久化 store 中的非终态任务标记为 failed，并附 `service_restarted_orphaned_task` 类原因，避免假性 `working`。

### Decision 5: Observability uses lightweight HTTP endpoints
`codex-a2a` 增加：
- `/healthz`：服务健康、启动时间、工作目录、输出目录、活跃任务数
- `/debug/tasks`：任务列表摘要
- `/debug/tasks/{task_id}`：单任务详情、阶段、最近更新时间、证据文件路径

主服务页面只展示摘要与远端 debug 链接，不直接镜像整份远端状态树。

## Risks / Trade-offs
- 文件 task store 会增加本地 I/O，但相比长任务状态丢失更可接受。
- 启动后将 orphaned 任务标记失败是显式暴露，不是真恢复；但这是比持续假装 `working` 更正确的行为。
- auto-recovery 可能带来额外轮询，需要受现有 tracking policy 控制。

## Migration Plan
1. 先在主服务修复 query path 与 tracker auto-recovery。
2. 再给 `codex-a2a` 加持久化 task store 与观测接口。
3. 最后把观测字段接到 settings 异步任务页面。
