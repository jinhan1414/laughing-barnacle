## Context
当前主干：
- `internal/llm/types.go` 提供 `llm.Client` 抽象，但实际仅有 `cerber` 实现。
- `cmd/server/main.go` 在启动时直接构造 `cerber.NewClient(...)` 并注入 Agent。
- `internal/config/config.go` 以 `CERBER_*` 为中心，配置与 provider 强绑定。

参考项目 `openclaw` 的 Codex 接入要点（本地克隆后分析）：
- API 类型层面：显式纳入 `openai-codex-responses`（`src/config/types.models.ts`）。
- 前向兼容层：为 `openai-codex/gpt-5.3-codex` 提供 fallback，含 `baseUrl=https://chatgpt.com/backend-api`（`src/agents/model-forward-compat.ts`）。
- 运行时策略层：
  - Codex 默认 transport `auto`（注释明确 WebSocket-first）（`src/agents/pi-embedded-runner/extra-params.ts`）。
  - `store=true` 仅对 direct OpenAI responses 强制，Codex 不强制（同文件注释与逻辑）。
- 认证层：将 `openai-codex` 作为独立 provider，使用 OAuth 资料进入统一 auth storage（`src/agents/pi-model-discovery.ts`, `src/agents/pi-auth-credentials.ts`, `src/agents/cli-credentials.ts`）。

## Goals / Non-Goals
- Goals:
  - 建立独立 LLM 网关边界，屏蔽 provider 协议差异。
  - 固化数字分身内部 LLM 格式，降低上下文与实现漂移。
  - 以 adapter 方式支持多 provider，并优先落地 Codex。
- Non-Goals:
  - 本次提案不要求立即拆成独立部署进程（先模块解耦）。
  - 不引入关键词/正则分流，不改变现有 Agent 业务决策链。
  - 不做静默 fallback 或 mock 成功路径。

## Decisions
### Decision 1: 新增 `internal/llmgateway` 作为唯一 LLM 入口
- Agent 只依赖网关接口，不直接依赖 `cerber` 或其他 provider 客户端。
- 网关负责：provider 选择、请求分发、统一错误归一、统一日志字段。

### Decision 2: 内部固定 Canonical LLM Contract
- 定义稳定内部格式（消息、工具调用、工具结果、usage、raw metadata）。
- Provider adapter 必须做双向映射；Agent 与上下层仅使用 Canonical 格式。
- 目标：降低 token 开销与解析歧义，保证“省 token 且稳定”。

### Decision 3: Provider Adapter 插件化
- 抽象 `ProviderAdapter`：
  - `Name()`
  - `Chat(ctx, canonicalReq) -> canonicalResp`
  - 可选能力探测（如 tool-calls、streaming transport）
- 网关通过 provider id + model 路由到 adapter，不在 Agent 层分支。

### Decision 4: Codex Adapter 采用显式特殊策略
- 支持 `openai-codex-responses` 语义。
- 默认 transport `auto`（WebSocket-first，SSE fallback）。
- 不强制 `store=true`（区别于 direct OpenAI responses 适配）。
- 支持 OAuth/API key 认证输入（实现阶段细化优先级）。
- 支持通过配置文件指定 Codex 认证文件路径（用于替代固定默认路径）。

### Decision 5: Codex 认证文件路径可配置
- 在网关 provider 配置中为 `openai-codex` 增加认证文件路径字段（命名实现阶段确定）。
- 当该字段存在时，Codex adapter 必须优先从该路径读取认证信息。
- 当该字段缺失时，才回退到默认路径策略。
- 当路径不可读或格式非法时，返回显式认证配置错误，禁止静默忽略。

### Decision 6: 配置从 provider 绑定转为网关中心
- 引入网关配置段（provider registry + default provider/model + timeout/retry）。
- 保持向后兼容迁移窗口：旧 `CERBER_*` 能映射到网关默认 provider 配置（迁移策略显式）。

## Risks / Trade-offs
- 风险：重构入口可能影响既有回复链路。
  - 缓解：先迁移 `cerber` 为 adapter，行为对齐后再引入 Codex。
- 风险：Codex 认证与 transport 差异导致回归失败。
  - 缓解：独立 adapter 测试 + 网关集成测试 + 错误直返。
- 风险：配置迁移期出现双配置冲突。
  - 缓解：定义优先级与启动期显式告警，不静默覆盖。
- 风险：Codex 认证文件路径配置错误导致不可用。
  - 缓解：启动期做路径可读性与格式预校验，失败时给出明确错误来源与路径。

## Migration Plan
1. 引入网关与 Canonical DTO，不改 Agent 业务流程。
2. 将 `cerber` 迁移为 adapter，确保行为与现网一致。
3. 接入 `openai-codex` adapter 与最小配置。
4. 增加 Codex 认证文件路径配置项与显式校验。
5. 启用配置迁移层（`CERBER_*` -> gateway default provider），输出显式告警。
6. 完成单测/集成测试后，逐步切换生产配置到网关结构。

## Open Questions
- 是否在本次实现阶段就要求“独立进程部署网关”，还是先完成“同仓独立模块”并预留 RPC/HTTP 边界。
- Codex 首版是否仅支持文本与工具调用，还是同步纳入图像输入能力。
