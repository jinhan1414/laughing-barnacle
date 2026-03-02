## 1. Specification
- [x] 1.1 增加 `autonomous-run-orchestration` 规格，明确 run 状态机、checkpoint 契约与 resume 触发条件
- [x] 1.2 通过 `openspec validate add-autonomous-run-orchestration --strict`

## 2. Implementation
- [x] 2.1 新增 autonomous run 数据模型、状态机、持久化与 conversation store 适配
- [x] 2.2 新增内置工具 `autonomous_run__checkpoint` 及参数校验
- [x] 2.3 新增 `context__read(resource=\"runs\", action=\"list|get\")`
- [x] 2.4 在 Agent 上下注入 run 索引，并增加 `resume_run` 专用上下文构造
- [x] 2.5 在 `async_task` 终态回调中触发 `resume_run`
- [x] 2.6 增加 `autonomous_run_status` 事件与聊天页运行状态展示
- [x] 2.7 设置页增加自主运行列表与步骤轨迹区域
- [x] 2.8 新增内置 Skill `autonomous-run-orchestrator`

## 3. Verification
- [x] 3.1 单测：`autonomous_run__checkpoint` 可创建与更新 run
- [x] 3.2 单测：非法 checkpoint 参数返回显式错误
- [x] 3.3 单测：`async_task` 终态可恢复等待中的 run
- [x] 3.4 单测：恢复执行时若模型未写 checkpoint，run 显式失败
- [x] 3.5 单测：聊天更新流包含 `autonomous_run_status`
- [x] 3.6 单测：设置页可查看自主运行列表与步骤轨迹
- [x] 3.7 单测：Skill 与 run 索引按渐进式披露注入
- [x] 3.8 回归：`go test ./... -timeout 60s`
