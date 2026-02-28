# Change: 根治 A2A 长任务跟踪超时误失败

## Why
当前 A2A 后台任务跟踪采用固定轮询窗口（2s * 45 = 90s），当远端任务仍处于 `working` 时会被本地直接标记为 `failed` 并写入 `a2a tracking timeout`。这会制造“假失败”，与真实远端状态不一致，且对长任务（代码分析、批处理、跨系统执行）稳定性不足。

此外，异步任务状态目前主要驻留内存，进程重启后无法可靠恢复未终态任务的跟踪上下文，导致可观测与可恢复性不足。

## What Changes
- 引入 A2A 异步任务持久化跟踪模型：将任务基础状态与跟踪状态（tracker metadata）持久化到数据库。
- 引入可恢复的后台跟踪调度器：服务启动后自动恢复未终态 A2A 任务跟踪。
- 调整终态判定：
  - 远端明确终态（`completed/failed/canceled/rejected`）才落本地终态；
  - 本地“轮询窗口耗尽/暂时不可达”不再直接标记 `failed`，而是保持任务 `working` 并显式标记“跟踪暂停”。
- `async_task__get` 增加按需回读对账：读取非终态 A2A 任务时执行一次远端状态对账并原子更新本地状态。
- 跟踪策略改为可配置（而非硬编码常量）：初始轮询间隔、最大间隔、最大跟踪时长、最大连续错误次数。
- 增加执行证据：区分“远端任务失败”与“本地跟踪暂停/中断”，保证排障可追溯。

## Impact
- Affected specs: `a2a-native-capability`
- Affected code:
  - `internal/agent/async_task_manager_a2a.go`
  - `internal/agent/async_task_manager.go`
  - `internal/agent/async_task_types.go`
  - `internal/agent/async_task_tools.go`
  - `internal/web/server_api_chat.go`（任务状态回读展示）
  - 异步任务持久化存储与恢复相关模块（新增）
