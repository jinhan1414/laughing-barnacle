## ADDED Requirements
### Requirement: Chat Greeting Requests Must Use Prompt Cache Session Under ChatGPT Auth
系统 MUST 在 ChatGPT OAuth 认证模式下为 `chat_greeting` 请求注入稳定缓存会话标识。

#### Scenario: Greeting request includes cache key and session header
- **WHEN** 适配器收到 `Purpose=chat_greeting`、`Model` 非空且认证模式为 ChatGPT OAuth 的请求
- **THEN** 请求 body 包含非空 `prompt_cache_key`
- **AND** HTTP header 包含与该键一致的 `session_id`

### Requirement: Prompt Cache Keys Must Be Purpose-Scoped And Stable
系统 MUST 为 `chat_reply` 与 `chat_greeting` 使用各自稳定且可复用的缓存键，并保持非目标 purpose 不注入缓存键。

#### Scenario: Chat reply key remains backward compatible
- **WHEN** 适配器处理 `Purpose=chat_reply` 请求
- **THEN** 继续使用当前既有稳定缓存键
- **AND** 不因本次变更改变主聊天缓存命名空间

#### Scenario: Greeting uses dedicated stable key
- **WHEN** 适配器处理 `Purpose=chat_greeting` 请求
- **THEN** 使用专用稳定缓存键
- **AND** 不与 `chat_reply` 共享同一缓存键

#### Scenario: Non-whitelisted purpose does not use cache key
- **WHEN** 适配器处理 `chat_reply`、`chat_greeting` 之外的 purpose
- **THEN** 请求 body 不包含 `prompt_cache_key`
- **AND** 请求 header 不包含 `session_id`

### Requirement: Greeting Prompt Prefix Must Remain Deterministic
系统 MUST 以固定顺序构造问候请求上下文（system/instructions/input 的相对顺序与字段标签固定），以保障缓存前缀稳定。

#### Scenario: Greeting request keeps fixed prompt structure
- **WHEN** 系统构造任意一次主动问候请求
- **THEN** system 指令与 user 模板字段标签顺序保持一致
- **AND** 仅运行时字段值变化，不改变模板结构
