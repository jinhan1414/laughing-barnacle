## ADDED Requirements
### Requirement: MCP JSON Save Supports Structured Env Payload
系统 MUST 允许 `POST /api/mcp/services/save` 为任意 MCP transport 接收结构化 `env` 对象，并对其进行显式校验与脱敏回读。

#### Scenario: Save MCP service with env map
- **WHEN** 调用 `POST /api/mcp/services/save`，且 `env` 为由非空字符串键值对组成的对象
- **THEN** 系统完成写入并返回 JSON 成功响应
- **AND** 后续 `GET /api/mcp/services` 回读可见该服务已配置环境变量的脱敏元数据
- **AND** 任意 JSON 响应都不回显 `env` 的真实值

#### Scenario: Reject malformed env payload
- **WHEN** `POST /api/mcp/services/save` 收到非对象 `env`、空键/空值或非字符串值
- **THEN** 系统返回明确错误状态与错误消息
- **AND** 不写入任何配置
