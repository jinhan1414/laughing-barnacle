---
name: "A2A 协议知识库"
description: "当用户询问 A2A / Agent2Agent 协议是什么、权威资料在哪里、与 MCP 的区别、Agent Card/Task/Message/Artifact 等核心概念、官方 SDK、Go/Python 接入方式，或需要结合本仓库解释 A2A 实战、联调与排障时使用"
---

目标：用“权威来源优先、代码现状优先、标准与实现分层”的方式回答 A2A 问题，避免把本仓库实现细节误说成协议标准。

先判断回答范围：
- 若用户问的是通用 A2A 协议，优先使用外部权威源。
- 若用户问的是本仓库里的 A2A 行为、接入、报错或链路，优先使用本仓库代码与测试。
- 若两者混在一起，先讲标准，再单独说明“本项目当前如何实现”。

外部协议权威源：
- 规范站：`https://a2a-protocol.org/latest/`。优先用来核对对象模型、协议字段、交互生命周期与传输约束。
- 官方仓库：`https://github.com/a2aproject/A2A`。优先用来核对规范演进、示例、讨论与版本背景。
- 官方 SDK：`https://github.com/a2aproject/a2a-go`、`https://github.com/a2aproject/a2a-python`。当用户问 Go/Python 如何接入时，以对应 SDK README 与 API 为准。
- Google 发布文章只用于说明协议背景，不作为规范性定义。

本仓库中的权威源与优先级：
- `openspec/changes/archive/2026-02-26-add-a2a-native-capability/`：说明为什么引入原生 A2A、Skill 与协议执行的边界。
- `integrations/codex-a2a/README.md`：本地 A2A 参考服务的启动、Agent Card 路径、任务生命周期与调试约束。
- `internal/a2a/provider_sdk.go`：当前后端如何使用官方 `a2a-go` SDK 做 Agent Card 发现、客户端创建、send/get/cancel 调用与 `skills` 校验。
- `internal/web/*`、`internal/agent/*`：当前产品里的维护入口、执行链路、索引注入与回复策略。
- 若文档与代码冲突，必须明确指出差异，并以当前主干代码与测试为准。

解释核心概念时遵循以下口径：
- A2A 是 Agent 与 Agent 的通信协议；MCP 是 Agent 与工具/上下文的连接协议。讲区别时用“Agent-to-Agent vs Agent-to-Tool/Context”表述，不要混为一谈。
- Agent Card 是 Agent 的能力与接入声明，重点关注身份、协议版本、`skills`、端点、鉴权与输入输出模式。
- Task 是可跟踪的工作单元；说明时优先强调 `task_id`、状态流转、结果/产物回读，而不是编造未核实的字段。
- Message/Part/Artifact 用于承载输入、分段内容与输出产物；如果用户要字段级示例，优先引用官方 schema 或当前代码，而不是凭记忆手写 JSON。
- 讲状态机时，优先依据官方文档或当前实现；若无法确认字段全集，使用“常见状态如 submitted / working / completed / failed / canceled”这类保守表述，并注明以官方 schema 为准。

解释实战时遵循以下流程：
- 先讲发现：读取 Agent Card，确认 `skills`、端点与鉴权。
- 再讲调用：发送请求、拿到任务或消息、按需查询任务、必要时取消。
- 最后讲证据：关注任务状态、最终消息、artifact、日志或事件流，而不是只看进程退出码。

针对本仓库，默认这样解释：
- A2A 接入维护走请求式链路：`/api/a2a/agents/save|toggle|delete`，并通过 `/api/a2a/agents*` 回读校验。
- A2A 执行走内置能力，当前后端通过官方 `a2a-go` SDK 调远端 Agent；参考服务 `integrations/codex-a2a` 通过官方 `a2a-python` SDK 暴露 `/.well-known/agent-card.json` 与 `/a2a/rpc`。
- 当前代码要求 Agent Card 必须有 `skills`；缺失或为空会被显式拒绝。
- 当前产品对长耗时 A2A 任务采用“进行中/错误直返，禁止同轮轮询扩张”的策略；解释执行问题时要把协议状态与产品编排策略分开说明。

回答本仓库 A2A 问题时，排查顺序固定为：
1. 触发条件是否命中正确的 A2A 维护或执行链路。
2. 动作计划是否选择了正确的 agent_id / skill / request。
3. 工具调用或 API 调用是否成功发出。
4. Agent Card、send/get/cancel、回读校验的返回是否与契约一致。
5. 最终回复是否忠实反映执行证据，而不是模型补写。

约束：
- 禁止把猜测说成规范。
- 禁止把本项目临时实现说成 A2A 标准要求。
- 禁止在没有 Agent Card、任务状态或 artifact 证据时声称“远端 Agent 已完成”。
