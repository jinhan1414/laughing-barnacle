## ADDED Requirements
### Requirement: Persistent Tracking for Long-Running A2A Tasks
系统 MUST 将 `task_type=a2a` 的后台任务及跟踪元数据持久化，并在进程重启后恢复未终态任务的跟踪。

#### Scenario: Persist A2A tracking metadata on submit
- **WHEN** 模型通过 `async_task__submit` 创建 `task_type=a2a` 任务
- **THEN** 系统持久化任务基础字段与跟踪字段（至少含 `task_id/status/agent_id/remote_task_id/tracker_state/next_poll_at`）
- **AND** 任一字段缺失或写入失败时返回显式错误

#### Scenario: Recover unfinished A2A trackers after restart
- **WHEN** 服务重启并加载本地任务存储
- **THEN** 所有未终态 A2A 任务都会恢复为可继续跟踪状态
- **AND** 不因进程重启直接将任务标记为失败

### Requirement: Tracking Window Exhaustion Must Not Produce False Failure
系统 MUST 在远端任务仍为进行中状态时避免将本地任务标记为 `failed`；轮询预算耗尽仅可转为“跟踪暂停”且保留业务状态为非终态。

#### Scenario: Polling budget exhausted while remote is still working
- **WHEN** 跟踪轮询达到当前预算上限且最近一次远端状态为 `submitted/working/running/pending`
- **THEN** 本地任务保持 `status=working`（或 `submitted`）
- **AND** 系统写入 `tracker_state=paused` 与显式原因（如 `tracking_window_exhausted`）

#### Scenario: Mark failed only when remote or protocol reaches terminal failure
- **WHEN** 远端返回 `failed/rejected` 或协议调用产生不可恢复终态错误
- **THEN** 系统将本地任务标记为 `failed`
- **AND** 错误证据明确区分“远端失败”与“本地跟踪暂停”

### Requirement: On-Demand Reconciliation via async_task__get
系统 MUST 在读取非终态 A2A 任务时执行一次远端状态对账，并将对账结果原子写回本地任务。

#### Scenario: async_task__get advances task to terminal state
- **WHEN** `async_task__get(task_id=...)` 命中非终态 A2A 任务且远端已完成
- **THEN** 系统在同次读取中将本地状态推进到对应终态
- **AND** 返回结果中包含更新后的状态与关键证据字段

#### Scenario: async_task__get keeps in-progress task with refreshed evidence
- **WHEN** `async_task__get` 命中非终态 A2A 任务且远端仍处于进行中
- **THEN** 系统返回非终态结果并刷新 `updated_at`/跟踪证据
- **AND** 不返回伪造终态

#### Scenario: async_task__get applies reconciliation debounce
- **WHEN** 连续触发 `async_task__get` 且距离上次对账未达到 `min_reconcile_interval`
- **THEN** 系统不发起远端对账请求并返回本地最新状态
- **AND** 返回结果包含可观测标记（如 `reconcile_skipped_by_debounce=true`）

### Requirement: Configurable Tracking Policy and Explicit Observability
系统 MUST 提供可配置的 A2A 跟踪策略与可观测证据，禁止使用不可调整的硬编码轮询窗口。

#### Scenario: Apply configured polling and retry policy
- **WHEN** 系统加载 A2A 跟踪配置
- **THEN** 跟踪逻辑按配置应用 `initial_interval/max_interval/max_tracking_duration/max_consecutive_errors/min_reconcile_interval`
- **AND** 配置非法时服务启动或加载阶段返回显式错误

#### Scenario: Auto-renew tracking window when max duration is reached
- **WHEN** 任务仍处于非终态且达到 `max_tracking_duration`
- **THEN** 系统自动续期跟踪窗口并继续维持任务非终态
- **AND** 记录续期证据（至少含续期时间与续期次数）

#### Scenario: Expose tracker transition evidence
- **WHEN** 跟踪状态发生变化（如 `active -> paused`、`paused -> recovering`、`recovering -> active`）
- **THEN** 任务日志记录状态迁移、触发原因与时间戳
- **AND** 设置页或查询接口可回读该证据用于排障
