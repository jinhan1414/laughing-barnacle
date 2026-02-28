# OpenAI Codex 提供商接入与上下文缓存审计说明

更新时间：2026-02-27  
适用仓库：`laughing-barnacle`

## 1. 文档目的

本文用于记录本仓库 `openai-codex` 提供商的真实实现细节，便于后续：

- 代码审核（实现是否与预期一致）
- 变更回归（修改后是否破坏缓存链路）
- 线上排障（缓存不命中/请求报错时快速定位）

## 2. 接入总览（当前实现）

LLM 网关在服务启动时注册两个提供商：`cerber`、`openai-codex`。  
`openai-codex` 由 `internal/llmgateway/adapters/openaicodex` 实现。

关键入口：

- 启动注册：[cmd/server/main.go](../cmd/server/main.go)
- 适配器主流程：[internal/llmgateway/adapters/openaicodex/adapter.go](../internal/llmgateway/adapters/openaicodex/adapter.go)

## 3. 配置项与认证优先级

### 3.1 环境变量

来自配置加载：[internal/config/llm_gateway_env.go](../internal/config/llm_gateway_env.go)

- `LLM_GATEWAY_OPENAI_CODEX_BASE_URL`
- `LLM_GATEWAY_OPENAI_CODEX_API_KEY`
- `LLM_GATEWAY_OPENAI_CODEX_AUTH_FILE_PATH`
- `LLM_GATEWAY_OPENAI_CODEX_TRANSPORT`
- `LLM_GATEWAY_OPENAI_CODEX_MAX_RETRIES`

示例见：[.env.example](../.env.example)

### 3.2 认证优先级（重要）

实现位置：[internal/llmgateway/adapters/openaicodex/auth.go](../internal/llmgateway/adapters/openaicodex/auth.go)

优先级固定为：

1. `AuthFilePath`（显式配置文件路径）
2. `APIToken`
3. 默认路径 `~/.codex/auth.json`

`auth.json` 支持两种来源：

- ChatGPT OAuth 结构（`auth_mode=chatgpt` + `tokens.access_token`）
- API key/token 字段（如 `OPENAI_API_KEY` / `api_key` / `token` 等）

## 4. 端点与请求构造

### 4.1 端点选择

实现位置：[internal/llmgateway/adapters/openaicodex/endpoint.go](../internal/llmgateway/adapters/openaicodex/endpoint.go)

- ChatGPT OAuth 模式默认走：`https://chatgpt.com/backend-api/codex/responses`
- API key 模式默认走：`https://api.openai.com/v1/responses`

### 4.2 输入消息格式（避免 400）

实现位置：[internal/llmgateway/adapters/openaicodex/payload.go](../internal/llmgateway/adapters/openaicodex/payload.go)

- `assistant` 消息映射为 `content.type = "output_text"`
- 其他角色映射为 `content.type = "input_text"`

这条映射用于规避 `input_text` 用在 assistant 上导致的 400 错误。

### 4.3 ChatGPT OAuth 模式额外行为

实现位置：[internal/llmgateway/adapters/openaicodex/payload.go](../internal/llmgateway/adapters/openaicodex/payload.go)

- 若无 system 指令，会注入默认 `instructions`
- 强制 `stream=true`
- 不显式设置 `temperature` 与 `transport`

## 5. 上下文缓存机制（核心）

### 5.1 缓存键生成规则

实现位置：[internal/llmgateway/adapters/openaicodex/prompt_cache_key.go](../internal/llmgateway/adapters/openaicodex/prompt_cache_key.go)

仅在以下条件同时满足时启用缓存键：

- `auth_mode=chatgpt`
- `Purpose == "chat_reply"`
- `Model` 非空

当前稳定会话键固定为：`agent:main:main`

### 5.2 请求中如何携带缓存会话

实现位置：

- body 写入 `prompt_cache_key`：[adapter.go](../internal/llmgateway/adapters/openaicodex/adapter.go)
- header 写入 `session_id`：[http.go](../internal/llmgateway/adapters/openaicodex/http.go)

即同一请求同时携带：

- `prompt_cache_key = "agent:main:main"`
- `session_id: agent:main:main`

### 5.3 命中行为（已验证）

真实调用结论（`gpt-5.3-codex`）：

- 短上下文：`cached_tokens` 可能持续为 0（看起来“不命中”）
- 长上下文：首轮冷启动后，后续轮次出现稳定 `cached_tokens>0`

结论：当前实现下缓存链路可生效，但命中可见性依赖输入体量与请求相似度。

### 5.4 审计口径说明

当前 `CanonicalTokenUsage` 不包含 `cached_tokens` 字段。  
`cached_tokens` 需从原始响应 `RawResponse.usage.input_tokens_details.cached_tokens`（或 `prompt_tokens_details.cached_tokens`）读取。

类型定义位置：[internal/llmgateway/canonical.go](../internal/llmgateway/canonical.go)

## 6. 审核检查清单（建议逐项勾验）

- [ ] `assistant -> output_text` 映射仍在
- [ ] `chat_reply + chatgpt auth` 时 body 含 `prompt_cache_key`
- [ ] 同条件下 header 含 `session_id`
- [ ] `prompt_cache_key` 为稳定会话键（当前固定 `agent:main:main`）
- [ ] 认证优先级未被破坏（AuthFilePath > APIToken > 默认文件）
- [ ] ChatGPT OAuth 与 API key 端点分流逻辑未被破坏
- [ ] 适配器相关测试全部通过

## 7. 回归验证命令

### 7.1 单元测试

```powershell
go test ./internal/llmgateway/adapters/openaicodex -timeout 60s -count=1
go test ./internal/llmgateway/... -timeout 60s -count=1
```

### 7.2 关键用例（缓存与消息类型）

```powershell
go test ./internal/llmgateway/adapters/openaicodex -run "TestAdapter_ChatReplyUsesStablePromptCacheKey|TestToResponsesInput_AssistantUsesOutputText|TestResolvePromptCacheKey" -timeout 60s -count=1 -v
```

## 8. 常见问题与定位

### 8.1 报错：assistant 内容类型非法

现象示例：`Invalid value: 'input_text'. Supported values are: 'output_text' and 'refusal'`

定位方向：

- 检查 `contentTypeForRole("assistant")` 是否仍返回 `output_text`
- 检查请求日志中 assistant 历史消息是否被错误改写

### 8.2 缓存看起来不命中

先检查：

- 是否为 `chat_reply` 场景
- 是否使用 ChatGPT OAuth 认证
- 请求中是否同时携带 `prompt_cache_key` 与 `session_id`
- 是否在短上下文/低重复度场景（此时 `cached_tokens` 可能为 0）

### 8.3 认证文件可用但服务报鉴权错误

先检查：

- `LLM_GATEWAY_OPENAI_CODEX_AUTH_FILE_PATH` 是否指向正确文件
- `auth.json` 中是否存在 `tokens.access_token`
- 响应是否为 401/403（会被映射为 `auth_config_invalid`）

## 9. 代码索引（审计入口）

- [cmd/server/main.go](../cmd/server/main.go)
- [internal/config/llm_gateway_env.go](../internal/config/llm_gateway_env.go)
- [internal/llmgateway/adapters/openaicodex/adapter.go](../internal/llmgateway/adapters/openaicodex/adapter.go)
- [internal/llmgateway/adapters/openaicodex/auth.go](../internal/llmgateway/adapters/openaicodex/auth.go)
- [internal/llmgateway/adapters/openaicodex/endpoint.go](../internal/llmgateway/adapters/openaicodex/endpoint.go)
- [internal/llmgateway/adapters/openaicodex/payload.go](../internal/llmgateway/adapters/openaicodex/payload.go)
- [internal/llmgateway/adapters/openaicodex/http.go](../internal/llmgateway/adapters/openaicodex/http.go)
- [internal/llmgateway/adapters/openaicodex/prompt_cache_key.go](../internal/llmgateway/adapters/openaicodex/prompt_cache_key.go)
- [internal/llmgateway/adapters/openaicodex/adapter_test.go](../internal/llmgateway/adapters/openaicodex/adapter_test.go)
- [internal/llmgateway/adapters/openaicodex/prompt_cache_key_test.go](../internal/llmgateway/adapters/openaicodex/prompt_cache_key_test.go)
