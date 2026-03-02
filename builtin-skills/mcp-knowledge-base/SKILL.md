---
name: "MCP 协议知识库"
description: "当用户询问 MCP / Model Context Protocol 是什么、官方规范与权威资料、与 A2A 的区别、host/client/server 架构、tools/resources/prompts/roots/sampling/elicitation、生命周期与 transport、官方 SDK、Inspector 调试方式，或需要结合本仓库解释 MCP 接入、调用、配置与排障时使用"
---

目标：用“官方规范优先、官方 SDK 次之、仓库实现再次之”的方式回答 MCP 问题，避免把本仓库当前实现误说成协议标准。

外部权威入口：
- 版本与当前 revision：`https://modelcontextprotocol.io/specification/versioning`
- 官方规范：`https://modelcontextprotocol.io/specification/2025-11-25`
- 官方文档首页：`https://modelcontextprotocol.io`
- 官方组织与 SDK：`https://github.com/modelcontextprotocol`

按需阅读，避免一次性加载全部引用资料：
- 需要确认权威来源与资料优先级时，读 `references/authority.md`。
- 需要解释 MCP 是什么、核心对象和与 A2A 的区别时，读 `references/concepts.md`。
- 需要解释协议版本、生命周期、transport、授权与安全边界时，读 `references/protocol-lifecycle-and-transport.md`。
- 需要解释官方 SDK、Inspector、服务端/客户端落地方式时，读 `references/sdk-and-debugging.md`。
- 需要解释“本仓库当前怎么实现 MCP”时，读 `references/repo-implementation.md`。

先判断回答范围：
- 若用户问的是通用 MCP 协议，优先使用 `modelcontextprotocol.io` 官方文档与规范。
- 若用户问的是 SDK 用法、调试与接入姿势，优先使用 `modelcontextprotocol` 官方 GitHub 组织下的 SDK/工具仓库。
- 若用户问的是本仓库里的 MCP 配置、调用、报错或链路，优先使用本仓库代码与测试。
- 若问题混合了“协议”与“本仓库现状”，必须先讲标准，再单独说明“本项目当前实现”。

统一口径：
- MCP 是 Host / Client / Server 协议，不是某个模型厂商私有 API；它解决的是“LLM 应用如何以标准方式连接工具与上下文”。
- 解释服务端能力时，优先按 `Resources / Prompts / Tools` 表述。
- 解释客户端能力时，优先按 `Roots / Sampling / Elicitation` 表述。
- 解释 transport 时，必须说明官方当前标准 transport 是 `stdio` 与 `Streamable HTTP`；`HTTP+SSE` 是旧版本协议路径，若提到 SSE，必须说明是兼容背景，不要当作当前标准结论。
- 解释授权时，必须区分：HTTP transport 可走 MCP 授权规范；`stdio` 通常不走该授权流，而是依赖本地环境。

回答本仓库问题时默认这样组织：
1. 先回答“官方标准上是什么”。
2. 再回答“本仓库现在怎么做”。
3. 若存在差异，单独列出“标准 vs 当前实现”。
4. 若涉及执行证据，优先引用配置回读、代码路径、测试断言与运行时日志。

针对本仓库的固定约束：
- 当前产品把 MCP 主要当作“外部工具提供方”，核心运行时围绕 `tools/list` 与 `tools/call`；不要默认本仓库已经完整消费了 `resources`、`prompts`、`sampling` 等所有协议能力。
- 当前 MCP 服务配置维护走 `/api/mcp/services/save|toggle|delete` 与 `/api/mcp/services` 回读。
- 当前仓库设置层允许 `streamable_http` / `sse` / `stdio` 三种 transport；解释时要明确：这是产品兼容能力，不等于官方最新标准只推荐这三者里的同等地位。
- 若文档与代码冲突，以当前主干代码和测试为准，并明确指出冲突点。

回答时禁止：
- 禁止把旧版本 HTTP+SSE 说成 MCP 当前标准 transport。
- 禁止把本仓库的 `sse` 配置项说成官方“当前推荐”。
- 禁止把 `tools`、`resources`、`prompts` 混成一个概念。
- 禁止在未核对协议版本时笼统声称“最新版就是某个旧 revision”。
