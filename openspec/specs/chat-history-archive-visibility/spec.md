# chat-history-archive-visibility Specification

## Purpose
TBD - created by archiving change update-chat-history-visibility-after-compression. Update Purpose after archive.
## Requirements
### Requirement: Compressed Conversation History Must Remain Reviewable
系统 MUST 在上下文压缩裁剪消息后，仍允许用户在聊天页回看被裁剪的历史对话内容。

#### Scenario: Review archived history after manual compression
- **WHEN** 用户在聊天页触发“压缩上下文”且历史消息被裁剪
- **THEN** 聊天页可见对应历史归档入口
- **AND** 用户可通过归档入口查看被裁剪对话内容

#### Scenario: Review archived history after automatic compression
- **WHEN** 系统在对话回合内自动触发压缩并裁剪历史消息
- **THEN** 用户刷新聊天页后仍可访问历史归档
- **AND** 历史回看不依赖再次向模型提问

### Requirement: Archive Sections Must Provide Complete Replay Content For New Records
系统 MUST 为新生成的归档分节持久化可回放的完整消息内容，不得仅保存截断摘要文本。

#### Scenario: Persist full content for trimmed messages
- **WHEN** 系统创建新的会话归档分节
- **THEN** 分节数据包含完整消息文本（至少含 `role` 与 `content`）
- **AND** 读取分节详情时可返回完整历史内容

### Requirement: Chat Archive Read APIs Must Support Progressive Disclosure
系统 MUST 提供聊天归档索引与分节详情的只读 API，并遵循“先索引后详情”的按需读取流程。

#### Scenario: Fetch archive index first
- **WHEN** 前端请求聊天归档索引
- **THEN** 接口返回归档标识、标题、摘要与时间信息
- **AND** 不返回全量分节正文

#### Scenario: Fetch section detail on demand
- **WHEN** 前端按 `archive_id` 与 `section_id` 请求归档分节详情
- **THEN** 接口返回对应分节的完整历史内容
- **AND** 参数非法或资源不存在时返回显式错误

### Requirement: Legacy Archives Must Be Readable With Explicit Incomplete Notice
系统 MUST 对旧格式归档保持可读，并在缺少完整消息载荷时显式提示内容可能不完整。

#### Scenario: Read legacy archive without full payload
- **WHEN** 前端读取旧格式归档分节
- **THEN** 系统返回当前可用内容
- **AND** 响应中包含“旧格式可能不完整”的显式标识

