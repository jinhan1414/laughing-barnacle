## ADDED Requirements
### Requirement: MCP JSON Save Supports Transport-Scoped Config Payload
系统 MUST 允许 `POST /api/mcp/services/save` 按 transport 接收结构化配置对象，并对其进行显式校验与脱敏回读。

#### Scenario: Save stdio MCP with env map
- **WHEN** 调用 `POST /api/mcp/services/save`，`transport=stdio`，且 `env` 为由非空字符串键值对组成的对象
- **THEN** 系统完成写入并返回 JSON 成功响应
- **AND** 后续 `GET /api/mcp/services` 回读可见该服务已配置环境变量的脱敏元数据
- **AND** 任意 JSON 响应都不回显 `env` 的真实值

#### Scenario: Save HTTP MCP with custom headers
- **WHEN** 调用 `POST /api/mcp/services/save`，`transport=streamable_http|sse`，且 `headers` 为由非空字符串键值对组成的对象
- **THEN** 系统完成写入并返回 JSON 成功响应
- **AND** 后续 `GET /api/mcp/services` 回读可见该服务已配置请求头的脱敏元数据
- **AND** 任意 JSON 响应都不回显 `headers` 的真实值

#### Scenario: Reject malformed or transport-mismatched config payload
- **WHEN** `POST /api/mcp/services/save` 收到非对象 `env` / `headers`、空键/空值、非字符串值、`stdio` 使用 `headers`、HTTP transport 使用 `env`，或 `headers` 试图写入 `Authorization`
- **THEN** 系统返回明确错误状态与错误消息
- **AND** 不写入任何配置
