## 1. Specification
- [ ] 1.1 补充 A2A 长任务持久化跟踪要求与恢复场景
- [ ] 1.2 补充“跟踪窗口耗尽不等于失败”的状态契约
- [ ] 1.3 补充 `async_task__get` 非终态回读对账要求
- [ ] 1.4 补充跟踪策略可配置与证据可观测要求
- [ ] 1.5 执行 `openspec validate refactor-a2a-long-task-tracking-persistence --strict`

## 2. Implementation
- [ ] 2.1 设计并实现异步任务持久化结构（任务基础状态 + tracker metadata）
- [ ] 2.2 在服务启动阶段恢复未终态 A2A 任务并重建跟踪调度
- [ ] 2.3 将固定轮询常量替换为可配置跟踪策略（初始间隔、最大间隔、最大时长、连续错误阈值）
- [ ] 2.4 重构 A2A 跟踪状态机：远端 in-progress 时禁止因本地轮询预算耗尽而置为 failed
- [ ] 2.5 为跟踪暂停/恢复/远端终态增加显式日志与错误码
- [ ] 2.6 在 `async_task__get` 中增加非终态 A2A 任务的按需远端对账更新
- [ ] 2.7 实现 `max_tracking_duration` 到期自动续期并记录续期审计字段
- [ ] 2.8 为 `async_task__get` 增加 `min_reconcile_interval` 防抖逻辑与可观测标记

## 3. Verification
- [ ] 3.1 单测：长任务超过单轮跟踪窗口后仍保持 working，且记录 tracking_paused 证据
- [ ] 3.2 单测：远端返回 completed/failed/canceled/rejected 时本地终态映射正确
- [ ] 3.3 单测：服务重启后可恢复未终态 A2A 任务并继续跟踪
- [ ] 3.4 单测：`async_task__get` 可触发非终态任务对账并推进状态
- [ ] 3.5 单测：跟踪策略配置边界校验生效（非法配置显式报错）
- [ ] 3.6 单测：达到 `max_tracking_duration` 时自动续期，任务保持非终态且记录续期证据
- [ ] 3.7 单测：`min_reconcile_interval` 防抖生效（窗口内不请求远端，窗口外恢复对账）
- [ ] 3.8 回归：`go test ./... -timeout 60s`
