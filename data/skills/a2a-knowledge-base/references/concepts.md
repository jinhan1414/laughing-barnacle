# A2A 核心概念

## 定位

- A2A 是 Agent 与 Agent 的通信协议。
- MCP 是 Agent 与工具、数据源、上下文能力的连接协议。
- 解释区别时，使用“Agent-to-Agent vs Agent-to-Tool/Context”口径。

## 常见对象

- Agent Card：能力声明与接入元数据，重点关注协议版本、身份、`skills`、端点、鉴权、输入输出模式。
- Task：可跟踪的工作单元，通常围绕 `task_id`、状态变化、最终结果与 artifact 展开。
- Message / Part：承载消息和分段内容的结构。
- Artifact：任务产物，适合表达最终输出、证据或补充内容。

## 状态说明

- 说明状态时，优先引用官方 schema 或当前实现。
- 无法确认全集时，可使用保守表述：常见状态如 `submitted`、`working`、`completed`、`failed`、`canceled`。
- 不要把示例状态当成协议保证的完整枚举。

## 解释模板

- 先说明对象用途。
- 再说明回答时真正需要关注的字段。
- 最后说明“当前问题中哪些字段需要以官方 schema 或代码为准”。
