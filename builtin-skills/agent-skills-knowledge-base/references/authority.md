# Agent Skills 权威资料与优先级

外部权威资料优先级：
1. `https://www.anthropic.com/news/skills`
   - 用来确认 Anthropic 官方对 Agent Skills 的定义、价值和典型结构。
2. `https://www.anthropic.com/engineering/agent-skills`
   - 用来确认 Anthropic 对“如何构建、触发、优化 Skill”的工程实践。
3. `https://agentskills.io/`
   - 用来确认当前开放 Agent Skills 规范与跨平台方向。
4. OpenAI 官方资料：
   - `https://openai.com/index/introducing-codex/`
   - `https://openai.com/index/introducing-upgrades-to-codex/`
   - `https://platform.openai.com/docs/mcp`
   - 这些资料用于说明 OpenAI/Codex 生态中对 `AGENTS.md`、Skills、Docs Skill 的产品实践与接入提示，不作为通用 Skill 规范的唯一来源。

本仓库内的权威资料优先级：
1. `internal/skills/store_persist.go`
2. `internal/skills/store_markdown.go`
3. `internal/skills/store_resources.go`
4. `internal/skills/store_core.go`
5. `internal/agent/reply_skill_index.go`
6. `internal/agent/context_read_tool.go`
7. `internal/web/server_api_skill_resources.go`
8. `internal/web/server_api_maintenance_manage.go`

使用规则：
- 讲“Agent Skill 是什么”，优先用 Anthropic 官方资料。
- 讲“开放格式/跨平台”，优先用 `agentskills.io`。
- 讲“OpenAI/Codex 里怎么体现 Skills”，只用 OpenAI 官方资料说明其产品实践。
- 讲“本仓库现在怎么跑 Skill”，以当前代码和测试为准。
- 若外部资料与本仓库冲突，先写外部通用概念，再写本仓库当前实现，不要混写。
