# 本仓库中的 MCP 现状

本仓库当前对 MCP 的定位：
- 主要把 MCP 当作“外部工具服务接入层”。
- 运行时核心围绕 `tools/list` 与 `tools/call`，而不是完整消费 MCP 全部能力。

关键代码路径：
- `internal/mcp/client.go`
  - 统一封装 MCP client，当前主要暴露 `ListTools` 与 `CallTool`。
- `internal/mcp/client_session_http.go`
  - HTTP transport 初始化、会话头、`MCP-Protocol-Version` 请求头、streamable/sse 分支。
- `internal/mcp/client_stdio_session.go`
  - `stdio` 子进程会话、初始化、复用与超时后重建。
- `internal/mcp/provider.go`
  - 从已启用服务收集工具并注入到 Agent。
- `internal/mcp/provider_call.go`
  - 根据工具绑定回调远端 MCP 服务。

配置与维护：
- 服务配置模型在 `internal/mcp/store.go`。
- 当前产品设置层支持三种 transport：
  - `streamable_http`
  - `sse`
  - `stdio`
- 配置校验在 `internal/mcp/store_validation.go`。
- 维护接口走：
  - `GET /api/mcp/services`
  - `POST /api/mcp/services/save`
  - `POST /api/mcp/services/toggle`
  - `POST /api/mcp/services/delete`

当前实现细节：
- 默认协议版本常量在 `internal/mcp/client.go`，当前是 `2025-06-18`。
- HTTP 调用初始化后，会继续发送初始化完成通知，并在后续请求携带 `MCP-Protocol-Version`。
- `stdio` 会按 `service.id` 复用会话，而不是每次调用都重启子进程。
- 设置页与存储层仍允许 `sse` transport，这代表产品兼容历史/特定实现，不代表官方最新标准推荐。

回答“这个仓库支持 MCP 的哪些部分”时，默认这样答：
- 已实现：MCP 服务配置、transport 连接、工具发现、工具调用、工具启停。
- 未见完整消费证据：`resources/list/read`、`prompts/list/get`、`sampling`、`elicitation` 在主执行链路中的系统化使用。

排障顺序：
1. 服务配置是否存在且启用。
2. transport、endpoint/command、env/headers 是否匹配。
3. 初始化与协议版本是否兼容。
4. `tools/list` 是否成功。
5. 目标工具是否启用。
6. `tools/call` 的返回与错误是否和远端服务一致。
