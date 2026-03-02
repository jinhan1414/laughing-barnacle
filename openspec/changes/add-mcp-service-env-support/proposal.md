# Change: 支持为 MCP 服务保存 transport 定向配置

## Why
当前 MCP 新增链路无法携带 transport 定向配置，导致 MCP 服务无法按标准 transport 语义完整保存与维护。

已确认的现状证据：
- `maintenance__write(resource="mcp", action="save")` 向 `/api/mcp/services/save` 发送 `env` 时，接口返回 `400`，错误为 `unknown field "env"`。
- `internal/web/server_api_maintenance_manage.go` 的 `apiMCPServiceSaveRequest` 不包含 `env` 字段。
- `internal/mcp/store.go` 的 `Service` 结构未持久化环境变量。
- `internal/mcp/client_stdio.go` 启动子进程时未注入服务级环境变量。
- `internal/mcp/client_session_http.go` 当前仅支持内建协议头与 `Authorization: Bearer <token>`，不支持额外自定义请求头。

这意味着：
- `stdio` MCP 无法通过产品内配置持久化 `env`
- HTTP 类 MCP 无法通过产品内配置传递 `Authorization` 之外的额外 header

## What Changes
- 为 MCP 服务新增 transport 定向配置能力，支持通过 JSON 维护接口与设置页保存：
  - `stdio` 使用 `env`
  - `streamable_http` / `sse` 使用 `headers`
- MCP 服务持久化时保存 `env` / `headers`，更新时支持“省略即保留、显式空对象即清空”的最小语义。
- MCP 列表回读仅暴露脱敏元数据（如 `has_env`、`env_keys`、`has_headers`、`header_keys`），不回显真实值。
- `stdio` MCP 启动子进程时注入已配置 `env`，形成可执行运行链路。
- HTTP 类 MCP 发起请求时附加已配置的自定义 `headers`，用于传递非 `Authorization` 的额外信息。
- 更新 MCP 维护 Skill / 运行时约束 / 测试，确保模型知道不同 transport 的字段边界。

## Impact
- Affected specs:
  - `maintenance-json-interfaces`
  - `mcp-service-config`（new）
- Affected code:
  - `internal/mcp/store.go`
  - `internal/mcp/store_services.go`
  - `internal/mcp/store_services_helpers.go`
  - `internal/mcp/store_persistence.go`
  - `internal/mcp/store_validation.go`
  - `internal/mcp/client_stdio.go`
  - `internal/mcp/client_session_http.go`
  - `internal/web/server.go`
  - `internal/web/server_api_catalog.go`
  - `internal/web/server_api_maintenance_manage.go`
  - `internal/web/server_settings_mcp_skill.go`
  - `internal/web/templates/settings.html`
  - `internal/skills/store.go`
  - `internal/mcp/*test.go`
  - `internal/web/*test.go`
  - `internal/skills/*test.go`
