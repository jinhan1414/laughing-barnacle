## Context
当前 `stdio` MCP 通过 `exec.CommandContext(ctx, ...)` 在每次 RPC 时启动子进程、握手并发送请求。该模式在 `npx tavily-mcp` 这类冷启动较重的工具上成本高，而且聊天入口使用整轮 `2m` 上下文，多个工具串行时会让最后一个请求在启动阶段直接吃到 `context deadline exceeded`。

## Goals / Non-Goals
- Goals:
  - 复用 `stdio` MCP 子进程，避免重复启动与重复初始化。
  - 让每次工具调用拥有独立的 2 分钟预算。
  - 在超时/异常后显式销毁失效会话，避免脏状态残留。
- Non-Goals:
  - 不引入新的 transport 或改动 MCP 配置协议。
  - 不把整轮聊天改造成后台任务。
  - 不修改 `bash` 20 秒默认超时和 LLM 网关超时。

## Decisions
### Decision 1: `stdio` 会话按 `service.id` 缓存在 MCP client 内
- 复用粒度与 HTTP `Mcp-Session-Id` 一致，保持运行时语义统一。
- 会话保存已启动进程、stdin encoder、stdout decoder、stderr buffer 与配置签名。
- 若配置签名变化（command/args/env），旧会话必须销毁并重建，避免“设置已改但仍用旧进程”。

### Decision 2: 单次 RPC 仍串行，但会话可跨多次调用复用
- 不引入并发多路复用；同一 stdio 会话内继续串行发送/读取 JSON-RPC，降低复杂度。
- 超时通过“请求级 ctx + 超时即关闭会话”实现。超时后该会话视为失效，下一次调用重建。

### Decision 3: 整轮 `/chat/send` 不再强加 2 分钟帽子
- 聊天入口直接传递 `r.Context()` 给 Agent。
- `callTool` 内部为每次工具调用创建 `2m` 子上下文。
- 这样整轮可持续多个工具调用，但每个工具的超时边界明确且独立。

## Risks / Trade-offs
- 风险：stdio 会话超时后强杀进程可能丢失会话内状态。
  - 缓解：stdio 本身无服务端 session 协议保证，超时后重建是更安全的一致性策略。
- 风险：整轮聊天不再有固定 2 分钟上限，极端情况下请求时长会增长。
  - 缓解：LLM 网关仍有独立超时；工具调用也有独立 2 分钟上限。

## Migration Plan
1. 新增 stdio 会话结构和缓存/失效逻辑。
2. 将 stdio RPC 改为“ensure session -> 复用会话请求 -> 错误时销毁重建”。
3. 将工具调用预算下沉到 Agent 的 `callTool`。
4. 移除聊天入口整轮 2 分钟限制。
5. 运行 OpenSpec 和 Go 测试验证。
