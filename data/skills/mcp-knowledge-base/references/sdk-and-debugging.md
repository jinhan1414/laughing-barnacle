# 官方 SDK 与调试工具

官方 SDK：
- TypeScript SDK：`https://github.com/modelcontextprotocol/typescript-sdk`
  - 适合解释 server/client 基础接入、示例工程和 docs。
- Python SDK：`https://github.com/modelcontextprotocol/python-sdk`
  - 适合解释 Python 生态中的 client/server 实现。
- Go SDK：`https://github.com/modelcontextprotocol/go-sdk`
  - 适合解释 Go 里如何创建 server、client 与 transport。
- 其他官方 SDK 可从 `https://github.com/modelcontextprotocol` 组织页确认。

SDK 相关固定口径：
- 先说明“官方 SDK 是实现协议的参考入口，不等于协议本身”。
- 需要讲协议字段时，以规范页为准。
- 需要讲代码写法、目录结构、示例和兼容性时，以官方 SDK 仓库为准。

当前常见官方结论：
- 官方 TypeScript SDK 文档明确支持 `stdio` 和 `Streamable HTTP`，并把旧 `HTTP + SSE` 标成 backwards compatibility only。
- 官方 SDK 通常覆盖：
  - tools / resources / prompts
  - client 连接与 server 暴露
  - 生命周期与 transport
  - 更高阶能力如 sampling、elicitation、tasks（是否稳定需看仓库说明）

调试与排障：
- 官方 Inspector：`https://modelcontextprotocol.io/docs/tools/inspector`
- 适合用于：
  - 校验服务端能否连接
  - 查看 capability negotiation
  - 浏览 resources/prompts/tools
  - 手工测试 tool 输入输出
  - 观察 notifications 与日志

回答“如何调试 MCP 服务”时优先建议：
1. 先用 Inspector 验证连接与 capability。
2. 再验证具体 feature：resources / prompts / tools。
3. 再检查 transport、鉴权、环境变量、请求头和协议版本。
4. 最后回到宿主应用内排查接入方实现差异。
