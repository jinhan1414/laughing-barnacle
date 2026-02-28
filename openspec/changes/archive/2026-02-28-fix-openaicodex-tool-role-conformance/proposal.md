# Change: 修复 OpenAI Codex 工具回合消息 role 兼容性

## Why
当前 `openai-codex` provider 在工具回合会把 `role=tool` 直接写入 Responses API 的 `input` 数组，导致请求被 Codex 拒绝并返回 400：
- `Invalid value: 'tool'. Supported values are: 'assistant', 'system', 'developer', and 'user'.`
- 触发点通常是“assistant 发起 tool_calls 后，下一条 tool 结果消息回注”这一轮。

这属于协议适配层的兼容性缺陷，会直接中断需要工具调用的会话。

## What Changes
- 在 `openai-codex` adapter 的输入归一化阶段增加角色白名单映射，确保发送给 Codex 的 `input[].role` 仅为 `assistant/system/developer/user`。
- 将 `tool`（以及其他未支持/空角色）统一映射为 `user`，保留原有消息内容，避免丢失工具执行结果上下文。
- 补充 adapter 级回归测试，覆盖 `tool` 角色与未知角色场景，防止再次回归到非法 role。

## Impact
- Affected specs:
  - `codex-tool-call-compatibility`（新增）
- Affected code (implementation stage):
  - `internal/llmgateway/adapters/openaicodex/payload.go`
  - `internal/llmgateway/adapters/openaicodex/payload_test.go`
  - （可选）`internal/llmgateway/adapters/openaicodex/adapter_test.go`
- Behavioral scope:
  - 仅影响 `openai-codex` provider 的请求序列化逻辑。
  - 不改变 Agent 工具编排策略，不改变其他 provider 的行为。
