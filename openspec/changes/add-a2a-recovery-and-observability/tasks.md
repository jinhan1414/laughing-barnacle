## 1. Spec
- [x] 1.1 新增 A2A 查询恢复与 codex-a2a 可观测性的 spec delta

## 2. Main Service
- [x] 2.1 调整 A2A provider：`GetTask/CancelTask` 优先使用 endpoint，降低 Agent Card 依赖
- [x] 2.2 增加 A2A tracker paused 后的自动恢复机制
- [x] 2.3 在 async task 结果/页面中展示阶段进度摘要与观测入口

## 3. codex-a2a
- [x] 3.1 引入持久化 task store，替换纯内存 task store
- [x] 3.2 服务启动时显式处理 orphaned 非终态任务
- [x] 3.3 增加 `/healthz`、`/debug/tasks`、`/debug/tasks/{task_id}` 观测接口

## 4. Validation
- [x] 4.1 补充 Go 测试覆盖查询链路与 paused 自动恢复
- [x] 4.2 补充 Python 测试覆盖 task store/观测接口
- [x] 4.3 运行 `go test ./... -timeout 60s`
- [x] 4.4 运行 `python -m pytest integrations/codex-a2a`
- [x] 4.5 运行 `openspec validate add-a2a-recovery-and-observability --strict`
