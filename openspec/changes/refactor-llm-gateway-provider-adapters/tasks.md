## 1. Specification
- [x] 1.1 新增 `llm-gateway-core` 规格，定义网关边界与内部固定格式
- [x] 1.2 新增 `openai-codex-adapter` 规格，定义 Codex 适配契约与特殊策略
- [x] 1.3 使用 `openspec validate refactor-llm-gateway-provider-adapters --strict` 校验通过

## 2. Implementation
- [x] 2.1 新增 `internal/llmgateway`（网关核心、adapter registry、provider 路由）
- [x] 2.2 定义 Canonical LLM DTO 与统一错误模型，替换 Agent 侧 provider 细节依赖
- [x] 2.3 将 `internal/llm/cerber` 迁移为 Cerber adapter，行为保持一致
- [x] 2.4 新增 OpenAI Codex adapter（`openai-codex-responses`、transport=auto、store 语义）
- [x] 2.5 增加 Codex 认证文件路径配置项，并在 adapter 中实现“显式路径优先”读取策略
- [x] 2.6 `cmd/server/main.go` 改为注入网关客户端而非直连 `cerber` 客户端
- [x] 2.7 配置层引入网关 provider 配置，并定义 `CERBER_*` 迁移映射与显式告警
- [x] 2.8 更新 `.env.example` 与 README 中的 LLM 配置说明

## 3. Verification
- [x] 3.1 单测：Canonical 请求/响应在 cerber adapter 映射正确
- [x] 3.2 单测：Codex adapter 的 transport/store 规则符合契约
- [x] 3.3 单测：Codex 认证文件路径可配置且优先级正确（显式路径 > 默认路径）
- [x] 3.4 单测：Codex 认证文件路径无效时返回显式错误（不可读/格式非法）
- [x] 3.5 单测：provider 路由、错误透传、重试边界符合网关规范
- [x] 3.6 集成：`go test ./... -timeout 60s` 通过
