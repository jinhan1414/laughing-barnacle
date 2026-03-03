# Change: add recoverable A2A tracking and codex-a2a observability

## Why
当前长时间 A2A 任务在远端 `codex-a2a` 进程短暂不可达或重启后，主服务会因连续轮询错误将 tracker 标记为 `paused`，且查询链路仍重新依赖 Agent Card，可导致任务“卡在 working 且无法自动恢复”。同时 `codex-a2a` 缺少健康与任务观测接口，用户无法判断服务是否在线、当前进行到哪一阶段。

## What Changes
- 调整主服务 A2A 查询链路：`GetTask/CancelTask` 优先直连已保存 endpoint，避免每次查询重新解析 Agent Card。
- 为 A2A tracker 增加 paused 后自动恢复机制，并保留明确的恢复/暂停证据。
- 为 `codex-a2a` 增加持久化 task store、启动后 orphaned 任务显式处理，以及健康/任务观测接口。
- 在后台任务视图中展示 A2A 进度摘要与远端观测入口，提升长任务可见性。

## Impact
- Affected specs: `a2a-native-capability`, `codex-a2a-reference-service`
- Affected code: `internal/a2a/provider_sdk.go`, `internal/agent/async_task_manager_*`, `internal/web/server*.go`, `internal/web/templates/settings.html`, `integrations/codex-a2a/*`
