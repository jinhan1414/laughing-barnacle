## ADDED Requirements
### Requirement: MCP Services Persist Transport-Scoped Secret Configuration
系统 MUST 为 MCP 服务按 transport 持久化受控配置，并在更新时保持确定性的替换语义。

#### Scenario: Omitted env preserves existing values
- **WHEN** 已存在带 `env` 的 `stdio` MCP 服务，且后续保存请求未包含 `env`
- **THEN** 系统保留该服务原有的环境变量集合
- **AND** 同次保存中的其他字段仍正常更新

#### Scenario: Empty env object clears existing values
- **WHEN** 已存在带 `env` 的 `stdio` MCP 服务，且保存请求显式传入 `env={}`
- **THEN** 系统清空该服务已保存的环境变量
- **AND** 后续回读结果显示该服务不再携带任何环境变量元数据

#### Scenario: Omitted headers preserves existing values
- **WHEN** 已存在带 `headers` 的 HTTP 类 MCP 服务，且后续保存请求未包含 `headers`
- **THEN** 系统保留该服务原有的请求头集合
- **AND** 同次保存中的其他字段仍正常更新

#### Scenario: Empty headers object clears existing values
- **WHEN** 已存在带 `headers` 的 HTTP 类 MCP 服务，且保存请求显式传入 `headers={}`
- **THEN** 系统清空该服务已保存的请求头
- **AND** 后续回读结果显示该服务不再携带任何请求头元数据

### Requirement: MCP Service Reads Redact Secret Values
系统 MUST 在 MCP 服务回读接口与设置页列表中仅暴露配置元数据，不暴露真实 secret 值。

#### Scenario: List service with configured env
- **WHEN** MCP 服务已保存 `env` 或 `headers` 且用户读取 `/api/mcp/services` 或基于该接口的列表视图
- **THEN** 返回结果仅包含是否已配置对应字段及键名信息
- **AND** 不返回任何配置值的真实内容

### Requirement: Stdio MCP Runtime Injects Configured Env
系统 MUST 在启动 `stdio` MCP 子进程时注入服务级环境变量，并与宿主环境合并后生效。

#### Scenario: Launch stdio MCP with configured env
- **WHEN** 系统为带 `env` 的 `stdio` MCP 服务创建子进程会话
- **THEN** 子进程可读取到已配置的环境变量
- **AND** 宿主环境中的基础变量仍然保留

### Requirement: HTTP MCP Runtime Injects Configured Custom Headers
系统 MUST 在 HTTP 类 MCP 请求中附加服务级自定义请求头，用于承接非 `Authorization` 的额外信息。

#### Scenario: Send HTTP MCP request with custom headers
- **WHEN** 系统为带 `headers` 的 `streamable_http` 或 `sse` MCP 服务发起请求
- **THEN** 请求包含已配置的自定义请求头
- **AND** `Authorization` 与协议必需头的生成逻辑保持独立
