## 1. Spec
- [ ] 1.1 完成 `maintenance-json-interfaces` 与 `mcp-service-config` 的 spec deltas。
- [ ] 1.2 运行 `openspec validate add-mcp-service-env-support --strict` 并修复全部问题。

## 2. MCP 数据与接口
- [ ] 2.1 为 `mcp.Service`、持久化与更新逻辑新增 `env` 字段，并实现“省略保留、空对象清空”的更新语义。
- [ ] 2.2 扩展 `/api/mcp/services/save` 与 `/settings/mcp/save`，支持所有 transport 的 `env` 输入并拒绝非法 payload。
- [ ] 2.3 扩展 `/api/mcp/services` 返回脱敏环境变量元数据，保证 `context__read(resource="mcp", action="list")` 可校验配置是否生效。

## 3. MCP 运行时与提示
- [ ] 3.1 在 `internal/mcp/client_stdio.go` 为子进程注入服务级环境变量，并保持宿主环境合并。
- [ ] 3.2 更新 MCP 设置页与维护 Skill 文案，明确 `env` 支持范围以及“留空不变，`{}` 清空”语义。

## 4. Validation
- [ ] 4.1 单测：保存不同 transport 的 MCP 时都可写入、保留、替换、清空 `env`。
- [ ] 4.2 单测：`env` 非对象、空键、空值、非字符串值等场景显式报错。
- [ ] 4.3 单测：`/api/mcp/services` 与相关回读链路仅返回 `has_env`/`env_keys`，不返回真实 secret。
- [ ] 4.4 单测：`stdio` MCP 子进程能读取到配置的环境变量。
- [ ] 4.5 回归 `go test ./... -timeout 60s`。
