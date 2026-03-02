## ADDED Requirements
### Requirement: MCP Services Persist Controlled Env Configuration
系统 MUST 为 `stdio` MCP 服务持久化受控环境变量，并在更新时保持确定性的替换语义。

#### Scenario: Omitted env preserves existing values
- **WHEN** 已存在带 `env` 的 `stdio` MCP 服务，且后续保存请求未包含 `env`
- **THEN** 系统保留该服务原有的环境变量集合
- **AND** 同次保存中的其他字段仍正常更新

#### Scenario: Empty env object clears existing values
- **WHEN** 已存在带 `env` 的 `stdio` MCP 服务，且保存请求显式传入 `env={}`
- **THEN** 系统清空该服务已保存的环境变量
- **AND** 后续回读结果显示该服务不再携带任何环境变量元数据

### Requirement: MCP Service Reads Redact Secret Values
系统 MUST 在 MCP 服务回读接口与设置页列表中仅暴露环境变量元数据，不暴露真实 secret 值。

#### Scenario: List service with configured env
- **WHEN** MCP 服务已保存环境变量且用户读取 `/api/mcp/services` 或基于该接口的列表视图
- **THEN** 返回结果仅包含是否已配置环境变量及键名信息
- **AND** 不返回任何环境变量的真实值

### Requirement: Stdio MCP Runtime Injects Configured Env
系统 MUST 在启动 `stdio` MCP 子进程时注入服务级环境变量，并与宿主环境合并后生效。

#### Scenario: Launch stdio MCP with configured env
- **WHEN** 系统为带 `env` 的 `stdio` MCP 服务创建子进程会话
- **THEN** 子进程可读取到已配置的环境变量
- **AND** 宿主环境中的基础变量仍然保留
