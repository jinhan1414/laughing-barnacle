# Change: 让聊天页主动问候复用 LLM 上下文缓存

## Why
当前主聊天链路（`Purpose=chat_reply`）在 ChatGPT OAuth 模式下会携带稳定 `prompt_cache_key/session_id`，可命中上下文缓存；
但聊天页主动问候链路（`Purpose=chat_greeting`）未进入同一缓存策略，导致同类请求长期冷启动，`cached_tokens` 难以提升。
这使“同一产品内相似固定前缀请求”的缓存行为不一致，也增加了问候链路的输入 token 成本。

## What Changes
- 新增“主动问候缓存一致性”能力：`chat_greeting` 在满足缓存前提时也必须携带稳定缓存会话标识。
- 保持 purpose 隔离：`chat_reply` 与 `chat_greeting` 使用不同稳定缓存键，避免跨用途串扰。
- 固化问候请求结构约束：保持 system/instructions/input 的固定顺序与固定字段标签，确保缓存前缀稳定。
- 增补回归验证：覆盖缓存键解析、请求体/请求头注入，以及非目标 purpose 不注入缓存键场景。

## Impact
- Affected specs: `chat-greeting-prompt-cache`
- Affected code:
  - `internal/llmgateway/adapters/openaicodex/prompt_cache_key.go`
  - `internal/llmgateway/adapters/openaicodex/adapter.go`
  - `internal/llmgateway/adapters/openaicodex/http.go`
  - `internal/llmgateway/adapters/openaicodex/*test.go`
  - `internal/agent/turn_handlers.go`（仅在需要时做结构稳定性微调）
  - `docs/openaicodex-provider-audit.md`
