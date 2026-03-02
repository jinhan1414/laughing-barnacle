# MCP 权威资料与优先级

外部权威资料优先级：
1. `https://modelcontextprotocol.io/specification/versioning`
   - 用来确认当前 protocol version 与 revision 状态。
2. `https://modelcontextprotocol.io/specification/2025-11-25`
   - 官方规范总入口；协议定义以这里及其子页面为准。
3. `https://modelcontextprotocol.io`
   - 官方实现文档、接入指南、Inspector 与开发说明。
4. `https://github.com/modelcontextprotocol`
   - 官方组织首页；用于确认官方 SDK、Inspector、Registry 等仓库归属。
5. 官方 SDK 仓库：
   - TypeScript: `https://github.com/modelcontextprotocol/typescript-sdk`
   - Python: `https://github.com/modelcontextprotocol/python-sdk`
   - Go: `https://github.com/modelcontextprotocol/go-sdk`
   - Java: `https://github.com/modelcontextprotocol/java-sdk`
   - Kotlin: `https://github.com/modelcontextprotocol/kotlin-sdk`
   - C#: `https://github.com/modelcontextprotocol/csharp-sdk`

本仓库内的权威资料优先级：
1. `internal/mcp/client.go`
2. `internal/mcp/client_session_http.go`
3. `internal/mcp/client_stdio_session.go`
4. `internal/mcp/provider.go`
5. `internal/mcp/provider_call.go`
6. `internal/mcp/store.go`
7. `internal/mcp/store_validation.go`
8. `internal/web/server_api_mcp_config_test.go`
9. `openspec/project.md`

使用规则：
- 讲“协议是什么”，以官方规范页为准。
- 讲“官方推荐怎么接”，以官方 SDK 与官方文档为准。
- 讲“这个仓库现在怎么跑”，以本仓库代码与测试为准。
- 若外部文档与本仓库实现不一致，先写“官方标准”，再写“本仓库当前实现”，不要混写。
