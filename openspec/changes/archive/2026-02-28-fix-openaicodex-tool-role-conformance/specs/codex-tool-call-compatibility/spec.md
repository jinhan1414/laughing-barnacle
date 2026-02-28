## ADDED Requirements
### Requirement: Codex Input Role Conformance
系统 MUST 在调用 OpenAI Codex Responses API 时，仅发送受支持的输入角色（`assistant/system/developer/user`）。

#### Scenario: Normalize tool role before sending request
- **WHEN** 上下文消息中存在 `role=tool`
- **THEN** adapter 在序列化请求前将该消息角色归一化为受支持角色
- **AND** 最终请求体中不得出现 `input[].role = tool`

#### Scenario: Fallback unknown roles to supported value
- **WHEN** 上下文消息角色为空或为未支持值
- **THEN** adapter 将其归一化为受支持角色后再发送
- **AND** provider 请求不因非法 role 被拒绝

### Requirement: Tool Result Context Preservation Under Role Normalization
系统 MUST 在角色归一化后继续保留工具结果消息内容，保证工具回合可延续。

#### Scenario: Keep tool output content after normalization
- **WHEN** assistant 产生工具调用且系统回注对应工具结果
- **THEN** 归一化后的输入仍包含该工具结果文本内容
- **AND** 不因角色修正而丢失工具执行结果上下文
