# Change: 修复上下文压缩后聊天页历史不可见

## Why
当前上下文压缩会裁剪会话消息，聊天页渲染仅基于当前 `messages/events`，导致用户刷新后看不到被裁剪的历史对话。
虽然裁剪内容已写入会话归档，但聊天页没有可用的读取与展示链路；同时归档段落内容存在截断，无法满足“完整历史回看”。

## What Changes
- 新增“压缩后历史可见性”能力，保证聊天页可回看被裁剪历史。
- 扩展会话归档数据契约，保留可回放的完整消息文本（而非仅摘要片段）。
- 新增聊天历史归档读取接口（索引 + 分节详情），保持按需读取，避免一次性加载全部归档。
- 聊天页新增“历史归档”展示区：
  - 显示归档索引（标题、摘要、时间）；
  - 点击后按需拉取分节并展示完整历史内容。
- 保持压缩策略不变：压缩仍用于降低推理上下文体积，不回退为“永不裁剪”。

## Impact
- Affected specs: `chat-history-archive-visibility`
- Affected code:
  - `internal/conversation/store_archive_build.go`
  - `internal/conversation/store_archive_read.go`
  - `internal/conversation/store.go`
  - `internal/web/server_chat.go`
  - `internal/web/server_api_chat.go`
  - `internal/web/server_setup.go`
  - `internal/web/templates/chat.html`
  - `internal/conversation/*test.go`
  - `internal/web/*test.go`
