## ADDED Requirements
### Requirement: Chat Send Shall Be Accepted Before Final Reply Completes
系统 MUST 在用户发送聊天消息时先完成“受理”，而不是等待数字分身整轮执行完成后才向前端返回。

#### Scenario: Return accepted response immediately after persisting user message
- **WHEN** 用户提交一条合法聊天消息
- **THEN** 系统先持久化该用户消息并分配 `message_id` 与 `turn_id`
- **AND** 立即返回受理结果（含 `accepted_at`）
- **AND** 不要求本次响应内已产出最终 assistant 回复

### Requirement: Chat Runtime Shall Use One Conversation With Queued Turns
系统 MUST 保持单主会话模型，并按提交顺序串行执行前台 turn，禁止为聊天主链路引入“主 session / 任务 session”双层会话结构。

#### Scenario: Multiple rapid user submissions are queued on the same conversation
- **WHEN** 用户在前一个 turn 尚未完成时连续提交多条消息
- **THEN** 系统仍在同一主会话下受理这些消息
- **AND** 为每条消息分配独立 `turn_id`
- **AND** turn 按提交顺序串行执行

### Requirement: Background Tasks Shall Run Concurrently Under Turn Association
系统 MUST 允许 turn 派生的后台 task 并发执行，并通过 `turn_id/task_id` 建立可追溯关联。

#### Scenario: One conversation has multiple in-progress tasks
- **WHEN** 不同 turn 或同一 turn 派生出多个后台 task
- **THEN** 系统允许这些 task 并发推进
- **AND** 每个 task 均可回溯到其所属 `turn_id`
- **AND** 不因 task 并发而改变主会话唯一性

### Requirement: Browser Realtime Delivery Shall Use SSE As Primary Channel
系统 MUST 为浏览器聊天页提供 SSE 实时事件通道，作为受理状态、执行进度与最终回复的主交付方式。

#### Scenario: SSE stream delivers ordered chat and task events
- **WHEN** 聊天页建立 SSE 连接且系统产生新的聊天执行事件
- **THEN** 服务端按顺序推送事件到浏览器
- **AND** 事件至少覆盖受理、turn 状态、task 状态与 assistant 消息

#### Scenario: SSE reconnect resumes from the last seen event cursor
- **WHEN** 聊天页 SSE 连接中断后按最后游标重连
- **THEN** 系统从该游标之后继续推送缺失事件
- **AND** 不重复丢失已确认顺序

### Requirement: Polling Fallback Shall Remain Available
系统 MUST 保留基于 cursor 的聊天增量读取接口，作为 SSE 不可用或需要补偿读取时的回退路径。

#### Scenario: Fallback polling restores missing updates when SSE is unavailable
- **WHEN** 浏览器无法建立或维持 SSE 连接
- **THEN** 前端仍可通过增量读取接口获取受理、状态和最终回复
- **AND** 用户无需手动刷新页面才能看到新状态

### Requirement: Chat UI Shall Render Optimistically And Show Execution Status
系统 MUST 在聊天页即时显示用户刚发送的消息，并在底部输入区附近展示数字分身当前执行状态与运行中任务摘要。

#### Scenario: User sees local message and running status immediately after submit
- **WHEN** 用户点击发送且消息通过本地校验
- **THEN** 聊天页立即显示该用户消息气泡
- **AND** 页面展示“已受理/排队中/执行中”等状态
- **AND** 最终 assistant 回复通过实时事件回填到时间线
