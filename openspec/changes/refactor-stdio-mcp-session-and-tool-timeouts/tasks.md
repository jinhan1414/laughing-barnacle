## 1. Spec
- [x] 1.1 新增 `mcp-runtime-execution` 规范，定义 stdio MCP 会话复用与失效重建行为
- [x] 1.2 新增 `agent-tool-execution-budget` 规范，定义单次工具调用 2 分钟预算
- [x] 1.3 通过 `openspec validate refactor-stdio-mcp-session-and-tool-timeouts --strict`

## 2. Runtime
- [x] 2.1 为 MCP client 增加 stdio 会话缓存与配置签名校验
- [x] 2.2 改造 stdio RPC 链路，复用已初始化子进程，并在超时/异常时销毁旧会话
- [x] 2.3 将单次工具调用预算下沉到 Agent `callTool`
- [x] 2.4 移除 `/chat/send` 与 `/chat/retry` 的整轮 2 分钟超时限制

## 3. Validation
- [x] 3.1 单测：stdio `ListTools + CallTool` 复用同一子进程
- [x] 3.2 单测：stdio 会话失效后可重建
- [x] 3.3 单测：Agent 对单次工具调用施加 2 分钟预算
- [x] 3.4 回归：`go test ./... -timeout 60s`
