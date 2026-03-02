## Context
当前聊天入口是同步表单提交：浏览器提交 `/chat/send` 后等待整轮处理完成，再重定向回聊天页。  
虽然系统已有后台任务、状态事件和 `/api/chat/updates` 增量读取，但前端在提交期间会暂停轮询，因此用户看不到“已受理”和“正在执行”的中间态。

同时，项目已有明确约束：
- 产品是单用户、全局单会话，不优先引入多 session 模型。
- 长任务能力已开始收敛到后台任务运行时。
- 浏览器聊天更新与 MCP `streamable_http` 不是同一层问题，不能混用。

## Goals / Non-Goals
- Goals:
  - 用户发送消息后立即看到自己的消息与“已受理”状态。
  - 通过实时事件持续看到数字分身正在执行、后台任务状态变化、最终回复。
  - 在单主会话下支持多个后台任务并发可见。
- Non-Goals:
  - 不把产品改造成多会话/多线程聊天产品。
  - 不把 MCP `streamable_http` 用作浏览器聊天实时传输。
  - 不替换现有后台任务编排语义。

## Decisions
### Decision 1: 运行模型固定为 `conversation -> turn -> task`
- `conversation`：唯一主聊天线程，对应当前全局单会话。
- `turn`：一次用户提交触发的一次前台处理单元，需分配稳定 `turn_id`。
- `task`：turn 派生的后台执行单元，可有 0..N 个，使用 `task_id` 跟踪。

该模型用于替代“主 session / 任务 session”的思路。  
会话只保留一个；并发发生在 task 层，不发生在主会话层。

### Decision 2: 发送链路改为“accepted-first”
新增聊天发送 API 契约：
- 先写入用户消息
- 分配 `message_id/turn_id`
- 将 turn 放入执行队列
- 立即返回受理结果

前端不再依赖整页重定向确认消息是否发送成功。

### Decision 3: SSE 作为浏览器实时交付主通道
聊天页新增 SSE 通道（如 `/api/chat/stream`），用于推送：
- `message.accepted`
- `turn.queued`
- `turn.working`
- `task.submitted`
- `task.working`
- `task.completed`
- `task.failed`
- `assistant.message`

选择 SSE 而不是 `streamable_http` 的原因：
- 浏览器侧主要是服务端单向推送；
- 前端实现简单，适合移动端聊天页；
- 与现有 `/api/chat/updates` 的 cursor 模型天然兼容。

### Decision 4: `/api/chat/updates` 保留为回退与补偿链路
SSE 不是唯一入口。系统仍保留 `/api/chat/updates`：
- 当浏览器、代理链路或网络环境不适合长连接时，前端可降级轮询。
- SSE 断线重连后，可按最后游标补拉缺失事件。

### Decision 5: 前台 turn 串行，后台 task 并发
由于主会话上下文是共享的，前台 turn 仍需串行执行，避免多轮同时推理污染同一上下文。  
但同一或不同 turn 派生的后台 task 可以并发执行，只通过事件与通知回写主聊天线程。

### Decision 6: 聊天时间线与任务状态区分层展示
聊天主时间线只展示对用户有意义的内容：
- 用户消息
- assistant 回复
- 精简状态事件

详细执行日志不直接灌入时间线，而是在底部状态区或任务详情中展示，避免聊天流被日志污染。

## Risks / Trade-offs
- 风险：turn 串行会让连续多条消息的最终回复顺序滞后。  
  缓解：先返回 `accepted`，并显式展示排队状态，保证交互不“卡住”。
- 风险：SSE 在部分代理链路上可能被中断。  
  缓解：保留 `/api/chat/updates` 作为回退与补偿读取。
- 风险：聊天时间线与任务状态区分展示后，前端状态管理复杂度上升。  
  缓解：统一事件类型与 cursor，避免前端自行推断状态。

## Migration Plan
1. 定义 `turn` 数据模型、状态机与事件结构。
2. 增加聊天发送“accepted-first”接口。
3. 增加 SSE 聊天事件流接口与 cursor 恢复契约。
4. 保留并对齐 `/api/chat/updates` 的回退语义。
5. 聊天页改为 optimistic UI + 实时状态栏。
6. 增加 turn 串行、task 并发、重连补拉、事件有序性的回归测试。
