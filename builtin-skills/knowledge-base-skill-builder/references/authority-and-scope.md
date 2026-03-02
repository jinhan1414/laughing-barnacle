# 外部权威来源与适用范围

构建我们这种知识库型 Skill 时，外部资料优先级建议如下：

1. Agent Skill 组织方法
   - Anthropic Skills 发布：`https://www.anthropic.com/news/skills`
   - Anthropic Agent Skills 工程实践：`https://www.anthropic.com/engineering/agent-skills`
   - Agent Skills 开放规范：`https://agentskills.io/`
2. 知识库本体与检索方法
   - OpenAI Retrieval：`https://platform.openai.com/docs/guides/retrieval`
   - OpenAI File search：`https://platform.openai.com/docs/guides/tools-file-search`
3. 外部知识源通过协议暴露给 Agent
   - OpenAI MCP：`https://platform.openai.com/docs/mcp/`

这些来源分别解决的问题：
- Anthropic / Agent Skills：定义 Skill 应该如何组织、触发、渐进披露。
- Retrieval / File search：定义文档型知识库如何被索引、检索、评估“是否适合向量检索”。
- MCP：定义外部知识源如何作为远端能力暴露给 Agent。

作用边界：
- “知识库型 Skill”不是“向量库说明文档”。
- “知识库型 Skill”也不是“任何知识都塞进 references”。
- 它的本质是：把一个主题的权威资料、概念分层、仓库实现和回答框架组织成可按需读取的 Skill。

本仓库内的权威来源优先级：
1. `internal/skills/store_markdown.go`
2. `internal/skills/store_resources.go`
3. `internal/skills/store_core.go`
4. `internal/agent/reply_skill_index.go`
5. `internal/agent/context_read_tool.go`
6. 目标主题对应的代码、测试、OpenSpec 与现有 Skill

使用规则：
- 讲“Skill 怎么写”，优先 Anthropic 与 Agent Skills。
- 讲“知识库怎么检索/索引”，优先 Retrieval / File search。
- 讲“外部知识源如何接到 Agent”，优先 MCP。
- 讲“本仓库怎么落地”，以当前代码和测试为准。
