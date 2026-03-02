# Skill 概念与边界

Skill 是什么：
- Anthropic 官方把 Agent Skills 定义为一种让模型按需调用的可复用专长包。
- 它不是只有一段提示词，而是把说明、资源、脚本和工作流经验一起打包。
- Anthropic 官方强调的几个特征包括：
  - 可组合
  - 可共享
  - 可移植
  - 上下文高效

典型形态：
- 一个 Skill 通常至少有一个 `SKILL.md`。
- `SKILL.md` 负责说明“这个 Skill 做什么、什么时候用、如何按需读取额外资源”。
- 复杂 Skill 往往再配合：
  - `scripts/`：确定性脚本
  - `references/`：按需加载的文档
  - `assets/`：产出时直接使用的模板或素材

Skill 与其他概念的边界：
- Skill vs Prompt：
  - Prompt 是一次性指令文本。
  - Skill 是长期可复用、可索引、可按需展开的能力包。
- Skill vs Tool：
  - Tool 是可执行接口或函数。
  - Skill 是关于“何时、如何组合调用工具与资源”的操作知识。
- Skill vs MCP：
  - MCP 是 Agent 与工具/上下文服务通信的协议。
  - Skill 是 Agent 内部复用知识与工作流的组织方式。
- Skill vs A2A：
  - A2A 是 Agent 与 Agent 通信。
  - Skill 是单个 Agent 内部可触发的专长单元。

触发口径：
- Anthropic 的表述是：Agent 会在上下文合适时调用相关 Skill，而不是必须由用户手动切换模式。
- OpenAI 官方 Codex 资料也把 Skills 视为 Codex 可利用的专门知识包，并与 `AGENTS.md`、项目级指令一起工作。

回答“什么时候该做 Skill”时可用这个口径：
- 当某类任务反复出现，而且它依赖固定知识、固定流程或固定脚本时，Skill 的收益最大。
- 这是基于 Anthropic 官方工程实践做出的归纳，不应说成某个硬性协议规则。
