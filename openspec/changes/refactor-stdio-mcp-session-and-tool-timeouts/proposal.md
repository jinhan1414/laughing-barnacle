# Change: 复用 stdio MCP 会话并将 2 分钟预算下沉到单次工具调用

## Why
当前 `stdio` MCP 每次 `tools/list` / `tools/call` 都会重新拉起子进程，`npx` 类工具冷启动成本高，容易在同一轮多次调用时耗尽聊天请求的总超时预算。与此同时，聊天入口把 `2m` 预算施加在整轮 `/chat/send`，导致“最后一个工具调用才暴露超时”，不符合用户希望的“单次工具调用 2 分钟”语义。

## What Changes
- 为 `stdio` MCP 引入按 `service.id` 复用的进程级会话，复用已初始化的子进程完成后续 `tools/list` / `tools/call`。
- 当 `stdio` 会话读写失败、超时或配置签名变化时，显式销毁旧会话并按需重建。
- 将 `2m` 超时从 `/chat/send` / `/chat/retry` 整轮请求下沉到单次 `callTool`，保证每次工具调用单独拥有 2 分钟预算。
- 保持 LLM 网关、`bash` 内置默认超时等现有边界不变，仅调整工具调用预算语义。
- 补充回归测试，覆盖 stdio 会话复用与工具级超时预算。

## Impact
- Affected specs: `mcp-runtime-execution`, `agent-tool-execution-budget`
- Affected code:
  - `internal/mcp/client.go`
  - `internal/mcp/client_stdio.go`
  - `internal/mcp/client_state.go`
  - `internal/agent/tool_call.go`
  - `internal/web/server_chat.go`
  - `internal/mcp/client_test.go`
  - `internal/agent/*test.go`
