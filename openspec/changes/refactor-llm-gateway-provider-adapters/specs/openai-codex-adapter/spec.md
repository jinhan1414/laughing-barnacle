## ADDED Requirements
### Requirement: OpenAI Codex Adapter Support
系统 MUST 支持 `openai-codex` provider，并通过 adapter 适配 `openai-codex-responses` 调用语义。

#### Scenario: Codex provider request is routed through dedicated adapter
- **WHEN** 模型引用为 `openai-codex/<model_id>`
- **THEN** 网关调用 Codex adapter
- **AND** 使用 Codex 对应的 API 语义映射而非复用通用 OpenAI 分支

### Requirement: Codex Transport Default
系统 MUST 将 Codex 默认 transport 设置为 `auto`（WebSocket-first，SSE fallback），并允许显式覆盖。

#### Scenario: Apply default transport when no runtime override is provided
- **WHEN** 调用 Codex 且未设置 transport 覆盖参数
- **THEN** adapter 发送 `transport=auto`
- **AND** 保留显式 runtime/config 覆盖能力

### Requirement: Codex Store Semantics
系统 MUST 不对 Codex responses 强制注入 `store=true`，并保持与 direct OpenAI responses 的差异化策略。

#### Scenario: Do not force store=true for Codex endpoint
- **WHEN** adapter 目标为 Codex responses API
- **THEN** 请求体不强制写入 `store=true`
- **AND** 相关策略差异在代码与测试中可追溯

### Requirement: Codex Credential Compatibility
系统 MUST 支持 Codex 所需认证凭据输入并显式处理认证失败。

#### Scenario: OAuth or token credentials are accepted for Codex
- **WHEN** 系统已配置 Codex 可用凭据（如 OAuth/token）
- **THEN** Codex adapter 可正常发起请求
- **AND** 缺失或失效凭据时返回显式认证错误

### Requirement: Configurable Codex Auth File Path
系统 MUST 允许用户在配置文件中指定 Codex 认证文件路径，并由 Codex adapter 按该路径读取认证信息。

#### Scenario: Use configured auth file path
- **WHEN** 配置中为 `openai-codex` 提供认证文件路径
- **THEN** Codex adapter 优先读取该路径
- **AND** 不依赖硬编码固定路径

#### Scenario: Invalid configured auth file path fails explicitly
- **WHEN** 配置路径不存在、不可读或内容格式非法
- **THEN** 系统返回显式认证配置错误
- **AND** 禁止静默回退为成功或隐藏错误
