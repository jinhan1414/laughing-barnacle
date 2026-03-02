---
name: "知识库 Skill 制作"
description: "当用户要求创建或更新我们这种 `SKILL.md + references/...` 结构的知识库型 Skill 时使用，尤其适用于协议知识库、产品能力知识库、概念澄清型 Skill、权威资料导读型 Skill，以及需要把外部权威资料与本仓库实现分层整理给数字分身的场景"
---

目标：把“知识库型 Skill”做成可检索、可渐进披露、权威优先、仓库现状分层的结构，而不是把一堆资料堆进单个 `SKILL.md`。

外部权威入口：
- Anthropic Skills 发布：`https://www.anthropic.com/news/skills`
- Anthropic Agent Skills 工程实践：`https://www.anthropic.com/engineering/agent-skills`
- Agent Skills 开放规范：`https://agentskills.io/`
- OpenAI Retrieval：`https://platform.openai.com/docs/guides/retrieval`
- OpenAI File search：`https://platform.openai.com/docs/guides/tools-file-search`
- OpenAI MCP：`https://platform.openai.com/docs/mcp/`

按需阅读，避免一次性加载全部引用资料：
- 需要确认外部权威资料优先级时，读 `references/authority-and-scope.md`。
- 需要设计知识库型 Skill 的结构时，读 `references/structure-and-progressive-disclosure.md`。
- 需要整理资料、挑选来源、分层写法时，读 `references/source-selection-and-writing.md`。
- 需要解释知识库与向量检索、MCP、仓库内 Skill 的边界时，读 `references/knowledge-base-boundaries.md`。
- 需要结合本仓库落地时，读 `references/repo-implementation.md`。

先判断这次要做的是哪类知识库：
- 协议/标准型：先找官方规范、版本页、官方 SDK，再补本仓库实现。
- 产品/平台型：先找官方产品文档、官方 API 文档、官方实践，再补本仓库接入。
- 仓库现状型：先读代码与测试，再决定是否补外部文档。
- 混合型：必须拆成“外部权威 / 核心概念 / 本仓库现状”至少三层。

知识库型 Skill 的固定结构：
1. `SKILL.md`
   - 只写触发条件、目标、资料分层、阅读顺序、回答框架、禁止事项。
2. `references/authority-*.md`
   - 写权威来源与优先级。
3. `references/concepts|protocol|sdk|repo-implementation|troubleshooting*.md`
   - 每个文件只承载一个主题层。
4. 如需可执行检查或格式转换，再加 `scripts/`；默认不要为了“看起来完整”乱加脚本。

制作步骤：
1. 先在线确认权威来源，优先官方站点与官方仓库。
2. 再确认本仓库现状，找代码、测试、接口与已存在 Skill。
3. 设计 `SKILL.md` 的最小正文，只保留：
   - 何时用
   - 先读哪个 reference
   - 回答时如何区分“标准 / 现状 / 差异”
4. 把细节拆进 `references/`，避免把大段资料塞回主正文。
5. 补 `internal/skills` 测试，至少验证：
   - 能出现在 Skill 索引里
   - `ReadEnabledSkillPrompt` 可读
   - 关键 reference 文件存在

默认写法约束：
- 外部事实若可能变化，必须写明资料入口或版本页。
- 不要把本仓库当前实现写成通用标准。
- 不要把平台实践写成跨平台强制规范。
- 不要把长文档原样搬进 `SKILL.md`。
- 不要无故新增 README、CHANGELOG、安装说明等噪音文件。

本仓库按需读取方式：
- 先读主正文：`context__read(resource="skills", action="read", id="<skill_id>")`
- 再读引用文档：`context__read(resource="skills", action="read", id="<skill_id>", path="references/<file>.md")`

回答“这个知识库 Skill 应该怎么组织”时，默认建议：
1. 主文件只做导航。
2. 细节拆 reference。
3. 外部权威与仓库现状分开写。
4. 先索引，后按需读取。
