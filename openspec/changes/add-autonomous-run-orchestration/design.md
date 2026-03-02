## Context
当前主干已经支持：
- `chat turn` 异步排队执行
- `async_task` 提交、查询、取消
- A2A 长任务回合外跟踪与终态通知

但系统缺少“多步目标”的主状态机。  
结果是后台任务完成后只能追加通知，无法自动决定下一步并继续执行。

## Goals / Non-Goals
- Goals:
  - 让数字分身在多步目标上具备事件驱动的自动续跑能力
  - 通过结构化 checkpoint 稳定保存 run 状态，避免依赖自然语言还原上下文
  - 保持低 token 与渐进式披露：恢复执行时只注入必要 run 上下文
  - 让用户在聊天页直接感知自主运行状态，设置页作为控制与排障面板
- Non-Goals:
  - 不引入新的独立消息队列或分布式调度器
  - 不把所有用户消息默认转为 autonomous run
  - 不引入关键词/正则分流

## Decisions
- Decision: run 状态与聊天 turn / async task 解耦
  - `chat turn` 仍负责“一条用户输入”的处理生命周期。
  - `async_task` 仍负责单个后台任务执行与跟踪。
  - `autonomous_run` 负责“多步目标”的状态机与恢复逻辑。

- Decision: 用显式 checkpoint 工具，而不是自然语言推断
  - 新增 `autonomous_run__checkpoint`。
  - 模型在“创建 run / 等待异步 / 等待人工 / 完成 / 失败”时必须调用该工具写入状态。
  - 理由：降低解析不稳定性，避免靠文案猜状态。

- Decision: `resume_run` 上下文采用结构化包
  - 默认注入：
    - 固定前缀
    - run 快照
    - 最新事件摘要
    - 最近步骤摘要
    - 工具 / A2A / async / run 索引
  - 详情按需读取，不默认注入全量日志与全文 artifact。

- Decision: async task 终态触发恢复
  - `async_task` 终态后，除现有通知外，还会检查是否存在 `waiting_async` 且 `waiting_ref=task_id` 的 run。
  - 命中后自动调用 `resume_run`。
  - 若恢复链路失败，run 显式转 `failed` 并记录错误原因。

- Decision: 用户主感知放在聊天页
  - 聊天页增加 `autonomous_run_status` 事件与 runtime status 摘要。
  - 设置页提供 run 列表和步骤轨迹，用于查看与排障。

## Risks / Trade-offs
- 风险：模型忘记调用 checkpoint。
  - Mitigation：runtime prompt 明确约束；恢复轮若缺 checkpoint，run 显式失败并记录证据。
- 风险：自主运行与普通聊天并发，可能引起消息交错。
  - Mitigation：run 状态通过事件流单独展示；assistant 通知保留时间顺序。
- 风险：上下文包过大。
  - Mitigation：只注入 run 快照、最近步骤摘要和索引；详细日志按需读取。

## Migration Plan
1. 定义 `autonomous_run` 数据模型、状态机、持久化接口与 conversation store 落盘。
2. 增加 `autonomous_run__checkpoint` 内置工具与 `context__read(resource="runs", ...)`。
3. 在 Agent 中注入 run 索引与恢复执行逻辑。
4. 将 `async_task` 终态回调接入 `resume_run`。
5. 扩展聊天页、设置页和 SSE 事件，展示自主运行状态。
6. 补单测、执行 `openspec validate` 与 `go test ./... -timeout 60s`。
