## ADDED Requirements
### Requirement: Independent LLM Gateway Boundary
系统 MUST 提供独立的 LLM 网关边界，作为 Agent 到外部 LLM Provider 的唯一调用入口。

#### Scenario: Agent uses gateway instead of provider client
- **WHEN** 服务启动并组装 Agent 依赖
- **THEN** Agent 注入的是网关接口实现
- **AND** Agent 不直接依赖任何具体 provider 客户端构造

#### Scenario: Gateway surfaces provider failure explicitly
- **WHEN** provider 调用失败（超时、认证失败、4xx/5xx）
- **THEN** 网关返回显式错误给上层
- **AND** 禁止静默 fallback 到其他 provider 或 mock 成功结果

### Requirement: Canonical Internal LLM Contract
系统 MUST 在数字分身内部使用固定 Canonical LLM 格式，不允许业务层直接耦合 provider 协议字段。

#### Scenario: Provider-specific fields are translated at adapter boundary
- **WHEN** Agent 发起一次对话请求
- **THEN** Agent 仅构造 Canonical 请求结构
- **AND** provider-specific 请求字段只在 adapter 内部生成

#### Scenario: Tool calls remain stable across providers
- **WHEN** 不同 provider 返回工具调用结果
- **THEN** 网关输出统一 Canonical tool-call 结构
- **AND** 上层工具执行链路无需按 provider 分支

### Requirement: Provider Adapter Registry
系统 MUST 通过可注册的 Provider Adapter 机制完成多 provider 适配。

#### Scenario: Route request by provider id
- **WHEN** 请求指定 provider id 与 model
- **THEN** 网关将请求路由到对应 adapter
- **AND** 未注册 provider 返回显式错误
