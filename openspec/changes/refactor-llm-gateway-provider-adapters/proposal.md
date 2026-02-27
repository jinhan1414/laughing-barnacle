# Change: 重构独立 LLM 网关并引入 Provider Adapter（含 OpenAI Codex）

## Why
当前系统把 LLM 调用直接绑定在 `internal/llm/cerber`，启动层也直接依赖 Cerber 配置，导致三个问题：
- 数字分身内部语义与外部 provider 协议耦合，后续新增 provider 成本高。
- 没有统一网关层，无法在一个稳定内部契约下做多 provider 适配与回归。
- Codex 这类 OpenAI 兼容但存在特殊行为（如 transport / store 语义）的 provider 难以低风险接入。

同时，对 `openclaw` 的接入链路分析显示其做法是“内部模型契约 + provider 适配层 + provider 特殊策略覆盖”，并已将 `openai-codex-responses` 作为一等 API 类型处理（见 design.md 中代码依据）。

## What Changes
- 新增独立 LLM 网关模块（同仓解耦），作为 Agent 与各 LLM Provider 之间的唯一调用入口。
- 定义数字分身内部固定 LLM 格式（Canonical Contract），Agent 仅依赖内部格式，不再依赖 provider 请求/响应细节。
- 引入 Provider Adapter 机制：每个 provider 通过 adapter 完成“内部格式 ↔ provider API”的双向转换。
- 首批 adapter：
  - `cerber`（保持现有能力，迁移到网关层）
  - `openai-codex`（新增）
- Codex adapter 采用参考实现语义：
  - 支持 `openai-codex-responses` 风格
  - 默认 transport 为 `auto`（WebSocket-first，SSE fallback）
  - 不对 Codex 强制 `store=true`
  - 支持在配置文件中显式指定 Codex 认证文件路径
- 配置层改为“网关配置优先”，Agent 仅保留内部模型选择字段。

## Impact
- Affected specs:
  - `llm-gateway-core`
  - `openai-codex-adapter`
- Affected code (implementation stage):
  - `internal/llm/*`（接口与调用入口重构）
  - `internal/llm/cerber/*`（迁移为 adapter 或被替代）
  - `cmd/server/main.go`（注入网关而非直连 provider）
  - `internal/config/config.go` 与 `.env.example`（网关配置、provider 配置、Codex 认证文件路径）
  - `internal/agent/*`（保持内部固定格式调用，不感知 provider 细节）
  - 新增 `internal/llmgateway/*`（核心网关与 adapters）
