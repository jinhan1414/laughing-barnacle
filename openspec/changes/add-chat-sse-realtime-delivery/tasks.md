## 1. Proposal
- [x] 1.1 明确 `conversation -> turn -> task` 运行模型与“非多 session”边界
- [x] 1.2 明确聊天发送 `accepted-first` API 契约与返回字段
- [x] 1.3 明确 SSE 主通道与 `/api/chat/updates` 回退/补偿协作关系
- [x] 1.4 明确前台 turn 串行、后台 task 并发的执行规则
- [x] 1.5 通过 `openspec validate add-chat-sse-realtime-delivery --strict`

## 2. Implementation
- [x] 2.1 新增聊天发送受理接口，返回 `message_id/turn_id/accepted_at`
- [x] 2.2 为聊天 turn 增加持久化状态与队列执行能力
- [x] 2.3 新增 SSE 聊天事件流接口与事件 cursor/重连语义
- [x] 2.4 保留 `/api/chat/updates` 并补齐与 SSE 一致的补偿读取逻辑
- [x] 2.5 聊天页改为 optimistic UI，不再依赖整页同步重定向确认发送结果
- [x] 2.6 聊天页新增底部执行状态区，展示 turn/task 进行中状态
- [x] 2.7 将后台任务状态与最终 assistant 回复统一映射为实时事件

## 3. Validation
- [x] 3.1 单测：用户消息受理成功后，无需等待最终回复即可返回
- [x] 3.2 单测：多个连续提交的 turn 按顺序执行
- [x] 3.3 单测：同一会话下多个后台 task 可并发推进并正确回写事件
- [x] 3.4 单测：SSE 断线后可按 cursor 补拉缺失事件
- [x] 3.5 单测：SSE 不可用时 `/api/chat/updates` 仍可完整恢复状态
- [x] 3.6 前端回归：用户消息立即显示，底部状态区可见进行中任务
