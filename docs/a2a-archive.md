# A2A（Agent2Agent Protocol）归档

更新时间：2026-02-26  
归档目的：沉淀 A2A 协议核心信息，供后续架构讨论与实现查询。

## 1. 一句话定义

A2A 是用于 **Agent 与 Agent 协作** 的开放协议，定义发现、认证、任务提交、状态跟踪、流式更新与异步回调等跨 Agent 交互标准。

## 2. 版本与演进快照

- 官方文档站点显示：`Latest stable draft: v1.0 RC`，并注明 `Latest released version: 0.3.0`。
- A2A GitHub `Releases` 当前可见最新发布：`v0.3.0`（发布时间：2025-08-12）。
- ACP（Agent Communication Protocol）已并入 A2A 生态：ACP 仓库于 2025-08-27 归档为只读；2025-08-15 官方讨论区已公告并入。

结论：当前需区分“**v1.0 RC（规范候选）**”与“**v0.3.0（已发布版本）**”，避免混用字段与方法名。

## 3. 核心模型（高频查询）

- `AgentCard`：Agent 对外能力描述与发现入口。
- `Task`：任务主对象，承载生命周期与状态。
- `Message`：回合消息（请求、澄清、补充输入）。
- `Artifact`：任务产物（文档、结构化结果、文件等）。
- `Part`：内容片段（文本、URL、结构化数据等）。

## 4. Task 生命周期（高频查询）

常见状态：

- `SUBMITTED`
- `WORKING`
- `INPUT_REQUIRED`
- `AUTH_REQUIRED`
- `COMPLETED`
- `FAILED`
- `CANCELED`
- `REJECTED`

终态（如 `COMPLETED/FAILED/CANCELED/REJECTED`）后不可“重开同一任务”；需求变化通常在同一上下文下创建新任务。

## 5. 协议操作（v1 文档主线）

常见方法族：

- `SendMessage`
- `SendStreamingMessage`
- `GetTask`
- `ListTasks`
- `CancelTask`
- `SubscribeToTask`
- Push 通知配置相关方法（创建/查询/列出/删除）
- `GetExtendedAgentCard`

补充：`SendMessage` 的返回语义是 `task` 或 `message` 二选一。

## 6. 发现、版本协商、扩展

- 发现入口：`/.well-known/agent-card.json`
- 版本头：`A2A-Version`（`Major.Minor`）
- 扩展头：`A2A-Extensions`
- 扩展可声明 required；若对端不支持 required 扩展，应报错而非静默降级。

## 7. 传输与异步语义

- 传输绑定：`JSON-RPC`、`gRPC`、`HTTP+JSON/REST`
- 流式：SSE 用于增量状态/结果
- 异步：Webhook Push 支持长任务与离线回传
- 交付语义：Push 按“至少一次”处理，客户端需做幂等

## 8. 安全要点

- 生产建议/要求使用 HTTPS（TLS）
- 认证信息放在传输层（如 Bearer、API Key、OAuth2、OIDC、mTLS）
- `AgentCard` 可签名（JWS + 规范化 JSON）用于信任校验

## 9. 与 MCP 的边界

- MCP：Agent/模型与工具、资源的连接协议
- A2A：Agent 与 Agent 的协作协议
- 推荐组合：**外层 A2A 编排协作，内层 MCP 调用工具**

## 10. 查询索引（给后续检索）

- 查“是什么”：看第 1 节
- 查“版本状态/兼容风险”：看第 2 节
- 查“对象模型”：看第 3 节
- 查“任务状态机”：看第 4 节
- 查“接口方法”：看第 5 节
- 查“发现/版本/扩展”：看第 6 节
- 查“传输与异步”：看第 7 节
- 查“安全与鉴权”：看第 8 节
- 查“MCP 关系”：看第 9 节

## 11. 官方来源（归档链接）

- A2A 文档首页：https://a2a-protocol.org/latest/
- What is A2A：https://a2a-protocol.org/latest/topics/what-is-a2a/
- 协议规范：https://a2a-protocol.org/latest/specification/
- v1 更新说明：https://a2a-protocol.org/latest/whats-new-v1/
- Task 生命周期：https://a2a-protocol.org/latest/topics/life-of-a-task/
- 流式与异步：https://a2a-protocol.org/latest/topics/streaming-and-async/
- 企业能力（安全等）：https://a2a-protocol.org/latest/topics/enterprise-ready/
- A2A 与 MCP：https://a2a-protocol.org/latest/topics/a2a-and-mcp/
- A2A GitHub：https://github.com/a2aproject/A2A
- A2A Releases：https://github.com/a2aproject/A2A/releases
- ACP 并入公告（LF AI & Data）：https://lfaidata.foundation/communityblog/2025/08/29/acp-joins-forces-with-a2a-under-the-linux-foundations-lf-ai-data/
- ACP 仓库（已归档）：https://github.com/i-am-bee/acp
- ACP 并入讨论公告（2025-08-15）：https://github.com/i-am-bee/acp/discussions/702
