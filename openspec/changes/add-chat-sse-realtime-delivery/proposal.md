# Change: 增加聊天即时受理与 SSE 实时交付能力

## Why
当前聊天页在用户发送消息后，会同步等待数字分身整轮处理完成才解除阻塞。  
这会让用户感知为“页面卡住”，与即时通讯式体验相反，也遮蔽了数字分身是否正在正常执行。  
系统已经具备后台任务与增量更新基础，但缺少“消息先受理、执行后续推送、单主会话下多任务并发可见”的前端交付契约。

## What Changes
- 新增“聊天即时受理”能力：
  - 用户发送消息时，服务端先持久化用户消息并返回 `accepted`，不等待最终 assistant 回复。
  - 返回字段至少包含 `message_id`、`turn_id`、`accepted_at`。
- 明确聊天运行模型为“单主会话 + turn 队列 + task 并发”：
  - 保持当前全局单会话，不引入“主 session / 任务 session”双层会话模型。
  - 前台 turn 按提交顺序串行执行，保证共享上下文一致性。
  - turn 派生的后台 task 可并发执行，并通过 `turn_id/task_id` 建立关联。
- 新增 SSE 实时交付通道：
  - 聊天页以 SSE 作为浏览器侧实时更新主通道，接收受理、执行中、后台任务状态、最终回复等事件。
  - 事件需具备有序 ID / cursor，支持断线重连与补拉。
- 保留现有 `/api/chat/updates`：
  - 继续作为 SSE 不可用时的回退与补偿读取路径。
  - 不把 `streamable_http` 引入浏览器聊天链路。
- 聊天页改为即时通讯式交互：
  - 用户消息本地立即显示。
  - 底部输入区展示数字分身当前执行状态与运行中任务摘要。
  - 最终 assistant 回复与后台任务终态通知通过实时事件回填到时间线。

## Impact
- Affected specs: `chat-realtime-delivery`
- Related changes:
  - `add-async-chat-task-orchestration`：提供后台任务能力与通知语义
  - 本提案补充聊天受理与实时交付层，不替代后台任务编排能力
- Affected code:
  - `internal/web/server_chat.go`
  - `internal/web/server_api_chat.go`
  - `internal/web/templates/chat.html`
  - `internal/agent/turn_handlers.go`
  - `internal/conversation/*`
  - `internal/agent/async_task_*`
  - `internal/web/*test.go`
