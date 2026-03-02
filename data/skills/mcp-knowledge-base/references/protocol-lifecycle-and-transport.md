# 协议版本、生命周期与传输

版本口径：
- 先查 `https://modelcontextprotocol.io/specification/versioning`。
- 若需要明确当前 revision，按该页的 `Current` 结论回答。
- 若需要解释变更历史，再看对应 revision 的 changelog。

截至 2026-03-02，官方版本页显示：
- Current protocol version 是 `2025-11-25`。
- `2025-06-18`、`2025-03-26` 等 revision 仍常见于实现和兼容说明中。

生命周期：
- MCP 基于 JSON-RPC 2.0。
- 初始化阶段要做 version negotiation 和 capability negotiation。
- 解释初始化时，优先使用 `initialize` 与初始化后的后续通知/请求这类说法；若用户追问字段级细节，再回到规范页核对。

transport 口径：
- 当前官方标准 transport 是：
  - `stdio`
  - `Streamable HTTP`
- 历史变化：
  - `2024-11-05` revision 中常见的是 `HTTP + SSE`
  - `2025-03-26` 起官方明确用 `Streamable HTTP` 替代旧的 `HTTP+SSE`
- 因此，若项目或 SDK 仍提到 SSE，通常是在讲兼容模式或实现细节，不应直接表述为“当前标准 transport 就是 SSE”。

`stdio` 解释模板：
- 客户端拉起本地子进程。
- 双方通过 stdin/stdout 交换 JSON-RPC 消息。
- stdout 只能写合法 MCP 消息；stderr 可用于日志。

`Streamable HTTP` 解释模板：
- 远端服务通过单个 MCP endpoint 处理 POST/GET。
- POST 用于发送 JSON-RPC 消息。
- 服务端可返回 JSON，也可使用 SSE 流式返回多条服务端消息。
- 这意味着“Streamable HTTP 可以内部借助 SSE 传输流式消息”，但 transport 名称仍是 `Streamable HTTP`。

授权与安全：
- 官方授权规范面向 HTTP transport。
- `stdio` 不应套用 HTTP 授权流，通常从本地环境读取凭据。
- 解释安全时优先强调：
  - 用户同意与控制
  - 工具调用风险
  - 数据最小暴露
  - HTTP 场景下的 Origin 校验、认证与本地绑定约束

回答 transport 争议时建议直接写：
- “官方当前标准 transport 是 `stdio` 与 `Streamable HTTP`；旧版 `HTTP+SSE` 已被替换，但一些实现仍保留 SSE 兼容路径。”
