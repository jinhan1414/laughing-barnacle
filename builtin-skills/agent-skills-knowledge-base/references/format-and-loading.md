# Skill 格式与加载方式

Skill 最小结构：
- `SKILL.md` 是核心入口。
- Anthropic 官方工程实践与开放规范都强调：Skill 应以一个清晰可发现的描述文件为中心。

`SKILL.md` 的职责：
- 用简洁 metadata 说明 Skill 名称和触发场景。
- 用正文说明工作流、边界和如何按需读取补充资源。
- 不应把所有细节一次性塞进正文。

渐进式披露：
- Anthropic 官方文章强调，上下文窗口是稀缺资源，Skill 设计要节省上下文。
- 因此 Skill 的推荐加载层次是：
  1. metadata：先让模型知道有哪些 Skill、各自负责什么。
  2. Skill 正文：命中后再读取 `SKILL.md`。
  3. 资源文件：只有在需要时再读 `references/` 或运行 `scripts/`。

推荐目录形态：
- `SKILL.md`
- `scripts/`
- `references/`
- `assets/`

开放规范口径：
- `agentskills.io` 将 Agent Skills 作为开放规范推进，目标是让 Skill 在多个 Agent 平台之间更可移植。
- 解释开放规范时，可说“开放规范支持比最小 `name/description/body` 更丰富的元信息与兼容性表达”；若用户要字段级细节，先回到规范页核对。

OpenAI/Codex 相关口径：
- OpenAI 官方 Codex 资料里可以看到 `AGENTS.md`、`Skills`、`Multi-agents` 等配置主题。
- OpenAI Docs MCP 文档明确建议在仓库放置 `AGENTS.md` 提示，并可配合 `OpenAI Docs Skill` 使用。
- 回答时要说清楚：这是 OpenAI 产品实践，不是所有 Agent 平台必须遵循的统一格式。
