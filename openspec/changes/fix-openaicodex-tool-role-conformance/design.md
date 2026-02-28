## Context
当前链路中，Agent 会在工具回合向 LLM 回注两类消息：
1. `assistant` 消息，携带 `tool_calls`。
2. `tool` 消息，携带对应工具执行结果。

`openai-codex` adapter 目前直接透传 `CanonicalMessage.Role` 到 Responses `input[].role`，没有做 Codex 角色约束适配。因此当出现 `tool` 消息时会触发 400 拒绝。

## Goals / Non-Goals
- Goals:
  - 消除 Codex 对 `role=tool` 的协议拒绝。
  - 不丢失工具结果文本上下文。
  - 保持改动最小、仅限 `openai-codex` adapter。
- Non-Goals:
  - 不重构 Agent 的工具调用流程。
  - 不在本次变更中切换到 `function_call_output` 结构化回传协议。
  - 不调整 system prompt 或业务策略。

## Decisions
### Decision 1: 引入 Codex 输入角色白名单映射
在 adapter 序列化阶段统一映射角色：
- `assistant/system/developer/user`：原样保留。
- `tool`：映射为 `user`。
- 空值或其他未知角色：映射为 `user`。

### Decision 2: 内容类型基于“映射后角色”计算
`content.type` 继续沿用现有规则：
- `assistant -> output_text`
- 其他角色 -> `input_text`

这样可避免出现“role 已合法但 content.type 与 role 语义冲突”的新偏差。

### Decision 3: 保持工具结果文本承载方式不变
本次仅修复协议兼容问题，不引入更大结构升级。工具结果仍以文本方式进入输入上下文，确保行为变化可控、验证范围可收敛。

## Risks / Trade-offs
- 风险：将 `tool` 映射为 `user` 可能弱化“工具来源”语义。
  - 缓解：先保证请求可被 Codex 接受；后续若需要更强语义再单独提案升级为结构化 `function_call_output`。
- 风险：只在 adapter 兜底，若未来新增角色仍可能出现语义漂移。
  - 缓解：通过白名单+默认 `user` 的策略避免请求级崩溃，并用单测锁定行为。

## Open Questions
- 当前提案阶段无阻塞性开放问题。
