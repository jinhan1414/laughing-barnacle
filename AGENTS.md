<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always open `@/openspec/AGENTS.md` when the request:
- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

Use `@/openspec/AGENTS.md` to learn:
- How to create and apply change proposals
- Spec format and conventions
- Project structure and guidelines

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

# Project Agents Notes

- 应用界面仅适配手机端。新增或修改前端页面时，按移动端优先设计与实现，不要求桌面端适配。
- Docker 镜像目标架构固定为 `linux/arm64`。新增或修改 Dockerfile、构建脚本、CI 配置时，默认保持 arm64，不切换为 amd64。
- Skill 技术方案固定为“渐进式披露”：首轮仅注入 skill 索引，不直接注入完整技能说明；需要详情时，只能通过 `linux__bash` 执行 `curl -s "http://127.0.0.1:8080/api/skills/read?id=<skill_id>"` 按需读取，不新增其他内置工具。
- 当用户明确提出“记住”某项长期规则时，必须同步更新到 `AGENTS.md` 后再回复确认；后续实现默认遵循已记录规则。
- 当前处于开发阶段，接受破坏性变更；上下文版本管理改为纯数据库方案，不再使用 Git 持久化。
- 完成任务后需要及时推送 Git（至少包含 `git commit` 与 `git push`），除非用户明确表示本次不要推送。
- 长上下文方案固定为：压缩时将被裁剪原文结构化归档（含标题与分节）写入数据库；摘要仅保留关键摘要与归档索引；需要详情时先取索引标题再按标题拉取具体分节内容，避免一次性注入全文。
- 内置 Skill 规范固定为：必须通过 Skills 存储层以标准 `SKILL.md` 形式管理（包含 frontmatter 的 `name`/`description` 与正文），并通过 Skill 索引做渐进式披露；禁止在 Agent 中硬编码整段技能提示词。新增或修改内置 Skill 时必须补充对应规范测试。
- 用户长期偏好：默认优先采用“省 token 且稳定”的实现方案（低上下文开销、结构清晰、便于 LLM 稳定解析）。
- 用户长期偏好：非必要不改动 system 文本/提示词；优先通过业务逻辑、状态管理与结构化上下文解决问题。仅在用户明确要求或确有必要时才调整 system 提示词。
- 用户长期规则：数字分身仅服务单个所属用户，产品采用全局单会话；方案设计默认支持该用户的长期记忆沉淀与持续进化（含项目信息持续维护）。
- 用户长期规则：Skill 调用机制以 LLM 主动触发为主（显式点名或按 skill description 隐式匹配）；默认不通过定时任务强制触发 Skill。
- 用户长期规则：核心业务机制禁止采用关键词/正则匹配分流（如按匹配结果决定是否注入上下文）；优先使用稳定的结构化状态或固定流程。
- 用户长期规则：聊天框底部工具栏作为统一扩展入口；用户可主动触发能力（如手动上下文压缩），后续新能力优先接入该入口并保持可扩展结构。
- 用户长期规则：后续问题或 bug 分析默认先从“数字分身执行链路”视角排查（触发条件→动作计划→工具调用→接口返回→回读校验→最终回复），优先定位执行证据与契约不一致点。
- 用户长期规则：废弃代码需要及时删除，保持仓库整洁，避免保留无效实现或不可达分支。
- 用户长期规则：单个代码文件行数不得超过 300 行；重构与新增代码默认按该约束拆分实现。
- 用户长期规则：关于定时任务等执行链路优化，固定采用“方案1（仅靠模型能力）”；不引入其他替代执行方式，保持能力随模型增强而增强。
- 用户长期规则：数字分身产品面向普通用户，默认应支持并优先处理日常化、口语化的自然指令，不要求用户具备专业技术背景。
- 用户长期规则：A2A 后续开发固定为“维护走请求式链路、执行走内置工具”：
  - A2A 接入维护（新增/修改/启停/删除）必须通过 `/settings/a2a/save|toggle|delete` + `/api/a2a/agents*` 回读校验，禁止新增或暴露 `a2a__register`。
  - 内置 A2A 工具仅保留 `a2a__send` / `a2a__get` / `a2a__cancel`，用于调用已接入 agent。
  - 设置页必须可见当前已接入 A2A 列表与状态，保证普通用户可直接查看与维护。
- 用户长期规则：当且仅当确认当前任务已完成时，必须且只发送一次 `curl -X POST "https://ntfy.sh/jincs5658944s" -d "done"`；未完成、失败或中断时禁止发送；发送后需在回复末尾写“已发送 done 通知”。
