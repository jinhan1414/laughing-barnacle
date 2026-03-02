# Change: 支持为 MCP 服务保存受控环境变量

## Why
当前 MCP 新增链路无法携带环境变量，导致带 `env` 的 MCP 服务配置无法在产品内完整保存与维护。

已确认的现状证据：
- `maintenance__write(resource="mcp", action="save")` 向 `/api/mcp/services/save` 发送 `env` 时，接口返回 `400`，错误为 `unknown field "env"`。
- `internal/web/server_api_maintenance_manage.go` 的 `apiMCPServiceSaveRequest` 不包含 `env` 字段。
- `internal/mcp/store.go` 的 `Service` 结构未持久化环境变量。
- `internal/mcp/client_stdio.go` 启动子进程时未注入服务级环境变量。

这意味着所有带 `env` 的 MCP 配置都会被当前维护接口拒绝；对于 Tavily 一类 `stdio` MCP，还会进一步卡在“即使保存成功，运行时也不会注入环境变量”的缺口上。

## What Changes
- 为 MCP 服务新增受控 `env` 配置能力，支持通过 JSON 维护接口与设置页保存。
- `env` 对所有 MCP transport 开放：`streamable_http`、`sse`、`stdio` 都可保存并回读脱敏元数据。
- MCP 服务持久化时保存 `env`，更新时支持“省略即保留、显式空对象即清空”的最小语义。
- MCP 列表回读仅暴露脱敏元数据（如 `has_env`、`env_keys`），不回显真实值。
- `stdio` MCP 启动子进程时注入已配置 `env`，形成当前可执行的完整运行链路；HTTP 类 transport 至少不得拒绝该配置。
- 更新 MCP 维护 Skill / 运行时约束 / 测试，确保模型知道 `env` 已是受支持字段。

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
  - `internal/web/server.go`
  - `internal/web/server_api_catalog.go`
  - `internal/web/server_api_maintenance_manage.go`
  - `internal/web/server_settings_mcp_skill.go`
  - `internal/web/templates/settings.html`
  - `internal/skills/store.go`
  - `internal/mcp/*test.go`
  - `internal/web/*test.go`
  - `internal/skills/*test.go`
