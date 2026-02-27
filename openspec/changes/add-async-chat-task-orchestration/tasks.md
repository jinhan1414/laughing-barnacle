## 1. Specification
- [ ] 1.1 明确“LLM 自主判定转后台”的决策边界与运行时约束
- [ ] 1.2 明确后台任务状态机与 `async_task__submit/get/cancel` 契约
- [ ] 1.3 明确后台任务索引固定注入规则（仅当天+运行中）与按需详情读取
- [ ] 1.4 明确设置页后台任务清单与全量日志可见性要求
- [ ] 1.5 明确 A2A 与后台任务一体化边界（移除 `a2a__*` 暴露 + 统一 async gateway）
- [ ] 1.6 通过 `openspec validate add-async-chat-task-orchestration --strict`

## 2. Implementation
- [ ] 2.1 新增后台任务运行时与存储（状态流转、错误记录、任务查询）
- [ ] 2.2 新增内置 Skill `async-task-orchestrator`（策略层）并补充规范测试
- [ ] 2.3 增加内置工具 `async_task__submit/get/cancel` 及参数校验
- [ ] 2.4 在 LLM 运行时提示中注入后台任务能力说明（由模型自主判定触发）
- [ ] 2.5 固定注入后台任务索引（仅当天+运行中）并支持按需读取任务详情
- [ ] 2.6 后台任务终态后唤起数字分身生成用户通知并写入会话
- [ ] 2.7 扩展 `/api/chat/updates` 与聊天时间线渲染，展示任务状态与通知
- [ ] 2.8 设置页新增后台任务清单与单任务全量日志展示
- [ ] 2.9 保持失败显式可见，禁止静默降级与通知链路递归
- [ ] 2.10 新增 A2A 类型后台任务执行器（回合外查询终态 + 终态通知）
- [ ] 2.11 下线模型侧 `a2a__send/get/cancel` 工具暴露并更新运行时提示

## 3. Verification
- [ ] 3.1 单测：内置 Skill `async-task-orchestrator` 注入与触发契约符合规范
- [ ] 3.2 单测：模型调用 `async_task__submit` 后返回合法 `task_id/status`
- [ ] 3.3 单测：`async_task__get/cancel` 对非法或不存在 task_id 返回显式错误
- [ ] 3.4 单测：每轮上下文仅固定注入“当天+运行中”后台任务索引，详情读取走按需路径
- [ ] 3.5 单测：后台任务成功/失败都会触发数字分身通知并带 task_id 证据
- [ ] 3.6 单测：`/api/chat/updates` 能增量返回任务状态事件与通知消息
- [ ] 3.7 单测：设置页可读取全部后台任务与全量日志
- [ ] 3.8 单测：A2A 任务统一经 `async_task__submit(type=a2a)` 承接并可回合外跟踪
- [ ] 3.9 单测：模型工具列表不再包含 `a2a__send/get/cancel`
- [ ] 3.10 单测：`async_task__submit` 缺失必填字段返回参数错误且不创建任务
- [ ] 3.11 单测：`task_type=a2a` 缺失 `agent_id/agent_input` 返回参数错误且不触发远端请求
- [ ] 3.12 单测：`async_task__get` 的 `log_limit` 上限校验生效
- [ ] 3.13 回归：`go test ./... -timeout 60s`
