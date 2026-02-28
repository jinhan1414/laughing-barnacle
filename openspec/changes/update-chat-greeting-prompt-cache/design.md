## Context
当前 `openai-codex` 适配器仅在 `Purpose=chat_reply` 时注入 `prompt_cache_key/session_id`。
主动问候由 `/chat/greet -> Agent.GenerateChatGreeting` 发起，`Purpose=chat_greeting`，因此不会命中该判定分支。
在聊天页长期使用场景下，问候请求具有高重复固定前缀，理论上应具备缓存复用价值。

## Goals / Non-Goals
- Goals:
  - 让 `chat_greeting` 在缓存前提满足时稳定携带缓存键。
  - 保持缓存键按 purpose 隔离，避免不同任务类型互相污染缓存命中。
  - 保证问候请求前缀结构稳定，减少因消息顺序漂移导致的 `cached_tokens=0`。
- Non-Goals:
  - 不把缓存策略扩展到所有 purpose。
  - 不改动问候业务规则（冷却时间、任务中不问候、fallback 文案）。

## Decisions
### Decision 1: 采用“白名单 purpose + 固定键”策略扩展缓存
在现有 `chat_reply` 基础上，新增 `chat_greeting` 为可缓存 purpose，其他 purpose 继续显式不注入缓存键。

### Decision 2: `chat_reply` 与 `chat_greeting` 使用不同稳定缓存键
- `chat_reply` 保持现有键（兼容当前主聊天缓存行为）。
- `chat_greeting` 使用独立稳定键（例如 `agent:greeting:main`）。
这样可避免不同请求语义共享同一缓存命名空间，降低互相稀释命中的风险。

### Decision 3: 固定问候请求前缀结构，不新增隐式回退
保持问候请求的 system 指令、user 模板字段顺序和标签固定，动态值只填充到既定槽位。
发生异常时继续显式报错/走现有 fallback，不引入静默降级分支。

## Risks / Trade-offs
- 风险：分键后缓存被拆分，单键样本量下降。
  - 缓解：问候链路自身模板高度稳定，分键有助于提高同类命中纯度。
- 风险：短上下文下即使注入缓存键，`cached_tokens` 仍可能为 0。
  - 缓解：将“已注入缓存键”与“实际命中 token 数”分开验收，避免误判实现失效。

## Migration Plan
1. 调整缓存键解析逻辑并补齐单测。
2. 回归问候链路请求结构稳定性测试。
3. 在开发环境连续触发问候，核对请求日志中 `prompt_cache_key/session_id` 与响应 usage。

## Open Questions
- 无。
