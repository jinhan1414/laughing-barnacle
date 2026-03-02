## 1. Spec
- [x] 1.1 完成 `maintenance-json-interfaces` 与 `mcp-service-config` 的 spec deltas。
- [x] 1.2 运行 `openspec validate add-mcp-service-env-support --strict` 并修复全部问题。

## 2. MCP 数据与接口
- [x] 2.1 为 `mcp.Service`、持久化与更新逻辑新增 `env` / `headers` 字段，并实现“省略保留、空对象清空”的更新语义。
- [x] 2.2 扩展 `/api/mcp/services/save` 与 `/settings/mcp/save`，支持 `stdio` 的 `env` 与 HTTP 类 transport 的 `headers` 输入，并按 transport 拒绝非法 payload。
- [x] 2.3 扩展 `/api/mcp/services` 返回脱敏配置元数据，保证 `context__read(resource="mcp", action="list")` 可校验配置是否生效。

## 3. MCP 运行时与提示
- [x] 3.1 在 `internal/mcp/client_stdio.go` 为子进程注入服务级环境变量，并保持宿主环境合并。
- [x] 3.2 在 `internal/mcp/client_session_http.go` 为 HTTP 类 transport 注入服务级自定义请求头，并禁止覆盖协议头与 `Authorization`。
- [x] 3.3 更新 MCP 设置页与维护 Skill 文案，明确 `env` / `headers` 的 transport 边界以及“留空不变，`{}` 清空”语义。

## 4. Validation
- [x] 4.1 单测：保存 `stdio` MCP 时可写入、保留、替换、清空 `env`。
- [x] 4.2 单测：保存 HTTP 类 MCP 时可写入、保留、替换、清空 `headers`。
- [x] 4.3 单测：`env` / `headers` 非对象、空键、空值、非字符串值、transport 不匹配、试图写 `Authorization` 等场景显式报错。
- [x] 4.4 单测：`/api/mcp/services` 与相关回读链路仅返回 `has_env`/`env_keys`/`has_headers`/`header_keys`，不返回真实 secret。
- [x] 4.5 单测：`stdio` MCP 子进程能读取到配置的环境变量，HTTP MCP 请求会附加自定义头。
- [x] 4.6 回归 `go test ./... -timeout 60s`。
