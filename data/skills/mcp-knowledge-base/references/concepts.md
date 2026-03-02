# MCP 核心概念

MCP 是什么：
- MCP（Model Context Protocol）是一个开放协议，用于让 LLM 应用以标准方式连接外部数据源和工具。
- 官方规范中的通信关系是 `Host -> Client -> Server`：
  - Host：承载 LLM 体验的应用。
  - Client：Host 内部连接某个 MCP server 的连接器。
  - Server：提供上下文或能力的服务。

优先这样解释能力边界：
- Server features：
  - `Resources`：只读上下文数据，按 URI 标识，可 `list/read`，可带模板、订阅和变更通知。
  - `Prompts`：可复用的提示模板，偏向用户显式选择和触发。
  - `Tools`：可执行函数，供模型调用，可能有副作用。
- Client features：
  - `Roots`：客户端暴露给服务端的可操作边界。
  - `Sampling`：服务端向客户端请求 LLM 采样。
  - `Elicitation`：服务端向用户追问额外信息。

区分原则：
- `Resources` 是读数据，不等于执行动作。
- `Prompts` 是模板化交互，不等于工具调用。
- `Tools` 是执行能力，不等于上下文资源。

与 A2A 的区别：
- MCP：Host/Client 与工具、上下文服务之间的协议，重点是“上下文与能力接入”。
- A2A：Agent 与 Agent 之间的通信协议，重点是“代理之间协作”。
- 回答区别问题时，使用“Agent-to-Tool/Context vs Agent-to-Agent”口径，不要混淆。

回答“为什么 MCP 重要”时可强调：
- 让不同宿主和不同服务端按统一协议互通。
- 降低工具/上下文接入对单一模型厂商或单一应用的绑定。
- 把上下文、工具、提示模板和客户端能力分层表达，便于安全控制与演进。
