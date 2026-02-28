## 1. Specification
- [ ] 1.1 新增 `chat-greeting-prompt-cache` 规范，定义问候链路缓存注入与 purpose 分键要求
- [ ] 1.2 定义问候请求结构稳定性约束（system/instructions/input 顺序与标签固定）
- [ ] 1.3 通过 `openspec validate update-chat-greeting-prompt-cache --strict`

## 2. Implementation
- [ ] 2.1 扩展 OpenAI Codex 适配器缓存键判定：在 ChatGPT OAuth 模式下支持 `chat_greeting`
- [ ] 2.2 为 `chat_greeting` 引入独立稳定缓存键，并保持 `chat_reply` 现有缓存键不变
- [ ] 2.3 补充/更新适配器单测：`prompt_cache_key` 与 `session_id` 注入、非目标 purpose 不注入
- [ ] 2.4 如有必要，微调问候请求构造以固定前缀顺序（不改变问候业务语义）
- [ ] 2.5 更新缓存审计文档，明确 `chat_greeting` 的缓存适用条件与排障口径

## 3. Verification
- [ ] 3.1 `go test ./internal/llmgateway/adapters/openaicodex -timeout 60s -count=1`
- [ ] 3.2 `go test ./internal/agent -run "TestGenerateChatGreeting_.*" -timeout 60s -count=1`
- [ ] 3.3 人工验证：连续触发问候请求时，日志请求体含 `prompt_cache_key` 且响应 usage 可观测 `cached_tokens`
