---
name: "Agent Skills 知识库"
description: "当用户询问 LLM Agent 的 Skill 是什么、Skill 与 prompt/tool/MCP/A2A 的区别、SKILL.md 结构、frontmatter、scripts/references/assets、渐进式披露、Skill 触发与组合、官方/开放规范、Anthropic Agent Skills、OpenAI Codex 中的 AGENTS.md 与 Skills 线索，或需要结合本仓库解释 Skill 存储、索引注入、按需读取与维护接口时使用"
---

目标：用“官方技能资料优先、开放规范次之、仓库实现再次之”的方式回答 Agent Skill 问题，避免把某个平台实现细节误说成通用标准。

外部权威入口：
- Anthropic 官方 Agent Skills：`https://www.anthropic.com/news/skills`
- Anthropic 工程博客：`https://www.anthropic.com/engineering/agent-skills`
- Agent Skills 开放规范：`https://agentskills.io/`
- OpenAI Codex 相关文章：
  - `https://openai.com/index/introducing-codex/`
  - `https://openai.com/index/introducing-upgrades-to-codex/`
  - `https://platform.openai.com/docs/mcp`

按需阅读，避免一次性加载全部引用资料：
- 需要确认资料优先级时，读 `references/authority.md`。
- 需要解释 Skill 概念、边界和与 prompt/tool/MCP/A2A 的区别时，读 `references/concepts-and-boundaries.md`。
- 需要解释 `SKILL.md`、frontmatter、资源组织和渐进式披露时，读 `references/format-and-loading.md`。
- 需要解释怎么写好 Skill、何时该抽 Skill、如何做评估与安全边界时，读 `references/authoring-evaluation-and-safety.md`。
- 需要解释本仓库里 Skill 如何存储、注入、按需读取与维护时，读 `references/repo-implementation.md`。

先判断问题范围：
- 若用户问的是“Skill 这个机制本身”，优先用 Anthropic 官方资料与 `agentskills.io`。
- 若用户问的是 OpenAI / Codex 生态里的 AGENTS.md、Skills、Docs Skill 线索，只用 OpenAI 官方文档说明其产品实践，不把它写成通用规范。
- 若用户问的是本仓库里的 Skill 行为、索引注入、资源读取或维护接口，优先用本仓库代码与测试。
- 若问题同时涉及“通用 Skill 概念”和“本项目当前实现”，先讲通用概念，再讲本仓库现状。

统一口径：
- Skill 是可复用的能力包，不只是“一段提示词”；它通常同时包含说明、脚本、参考资料和触发线索。
- 回答 Skill 结构时，优先用 `SKILL.md + scripts/ + references/ + assets/` 这套组织方式。
- 解释加载策略时，必须强调渐进式披露：先 metadata，再 Skill 正文，再按需读引用资源。
- 解释触发时，优先说“模型基于语义自动判断是否命中”，而不是把 Skill 描述成必须人工切换的模式。
- 解释边界时，要把 Skill、Tool、Prompt、MCP、A2A 分开，不要混成一个抽象。

回答本仓库问题时默认这样组织：
1. 先回答“通用 Agent Skill 上是什么”。
2. 再回答“本仓库当前怎么实现 Skill”。
3. 若存在差异，单独列出“通用规范/平台实践 vs 当前实现”。
4. 若涉及执行证据，优先引用 `SKILL.md`、资源索引、代码路径、API 返回与测试。

针对本仓库的固定约束：
- 当前仓库将 Skill 作为本地文件夹能力包；仓库内置 Skill 真源位于 `builtin-skills/<skill_id>/SKILL.md`，运行时目录仍是 `data/skills/<skill_id>/SKILL.md`。
- 当前主链路是“先注入 Skill 索引，再按需 `context__read(resource=\"skills\", action=\"index|read\")` 读取详情”，而不是首轮把全部 Skill 正文注入上下文。
- 当前解析层稳定依赖 `name` / `description` 与正文，不应把开放规范中的其他潜在字段默认说成本仓库已支持。
- 若外部资料与代码冲突，以当前主干代码和测试为准，并明确指出差异。

回答时禁止：
- 禁止把 Skill 简化成“只是 system prompt 别名”。
- 禁止把平台实践写成跨平台强制标准。
- 禁止把本仓库的 `SKILL.md` 解析能力夸大成已完整支持整个开放规范。
- 禁止在未核对权威来源时声称“所有 Agent 平台都以同一种 Skill 格式工作”。
