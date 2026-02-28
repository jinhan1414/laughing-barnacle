## Context
当前 A2A 异步跟踪逻辑以固定轮询窗口执行，窗口耗尽即落 `failed(a2a tracking timeout)`。该策略无法区分“远端真实失败”与“本地追踪预算耗尽”，对长任务产生误判。

同时，异步任务运行时以内存态为主，进程重启会丢失未终态任务的跟踪上下文，不满足长链路可恢复要求。

## Goals / Non-Goals
- Goals:
  - 消除长任务“假失败”：远端仍在运行时，本地不得因追踪预算耗尽而终态失败。
  - 提供持久化与重启恢复能力，确保 A2A 长任务可持续跟踪。
  - 保持失败显式暴露：区分远端失败、本地跟踪暂停、通信中断。
  - 控制资源开销：通过可配置策略降低高频轮询压力。
- Non-Goals:
  - 不改变 A2A 协议调用入口（仍走 `async_task__submit/get/cancel`）。
  - 不引入关键词/正则分流策略。

## Decisions
- Decision: 拆分“业务终态”与“跟踪态”
  - `task.status` 仅表达业务终态（submitted/working/succeeded/failed/canceled）。
  - 新增 `tracker_state` 与 `tracker_reason`（如 `active/paused/recovering`、`tracking_window_exhausted`）。
  - 理由：避免将调度状态污染为业务失败，提升语义稳定性。

- Decision: A2A 任务持久化 + 启动恢复
  - 持久化字段至少包含：`task_id/task_type/status/agent_id/remote_task_id/updated_at/tracker_state/tracker_reason/next_poll_at/consecutive_errors`。
  - 服务启动后扫描未终态 A2A 任务并重建跟踪 worker。
  - 理由：保证重启后链路连续性与可追踪。

- Decision: 轮询策略参数化并采用退避
  - 将固定 `2s*45` 替换为策略对象：`initial_interval/max_interval/max_tracking_duration/max_consecutive_errors`。
  - 对连续可重试错误采用退避，超过阈值转 `tracker_state=paused`，不直接 `failed`。
  - 理由：减少无效轮询，避免暂时网络故障造成误判。

- Decision: `async_task__get` 触发非终态对账
  - 对 `task_type=a2a` 且 `status` 非终态任务，在 `get` 请求时执行一次远端状态探测并原子更新。
  - 理由：用户/模型主动查询可获得最新状态，缩短“暂停后状态滞后”窗口。

## Risks / Trade-offs
- 风险：状态机复杂度上升。
  - Mitigation：以状态转移表和单测覆盖关键路径（远端终态、暂停、恢复、重启）。
- 风险：持久化读写带来额外 I/O。
  - Mitigation：仅在状态变化或日志关键事件时落盘，避免每次轮询全量写入。
- 风险：暂停任务长期堆积。
  - Mitigation：暴露明确 `tracker_state/tracker_reason`，并提供 cancel/手动触发 get 的恢复路径。

## Migration Plan
1. 增加异步任务持久化 schema（向后兼容读取旧任务）。
2. 上线新状态机与可配置策略，默认开启恢复调度器。
3. 将旧的 `a2a tracking timeout -> failed` 行为迁移为 `working + tracker_state=paused`。
4. 增量补齐日志与设置页可观测字段，便于线上验证。

## Open Questions
- `max_tracking_duration` 到达后是否允许自动续期，或仅转暂停等待用户动作。
- `async_task__get` 对账频率是否需要最小间隔防抖（避免高频刷远端）。
