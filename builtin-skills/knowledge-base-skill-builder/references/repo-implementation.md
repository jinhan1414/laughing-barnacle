# 本仓库中的落地方式

本仓库当前 Skill 结构：
- 仓库内置 Skill 真源目录：`builtin-skills/<skill_id>/`
- 运行时 Skill 目录：`data/skills/<skill_id>/`
- 主文件：`SKILL.md`
- 可选资源：`references/*.md`、`scripts/*`

当前注入链路：
- `internal/agent/reply_skill_index.go`
  - 首轮只注入 Skills 索引，不注入全部正文
- `internal/agent/context_read_tool.go`
  - 按需读取：
    - `context__read(resource="skills", action="list")`
    - `context__read(resource="skills", action="index", id="<skill_id>")`
    - `context__read(resource="skills", action="read", id="<skill_id>")`
    - `context__read(resource="skills", action="read", id="<skill_id>", path="references/<file>.md")`

当前解析边界：
- `internal/skills/store_markdown.go`
  - 稳定读取 frontmatter 中的 `name`、`description`
- `internal/skills/store_resources.go`
  - 资源索引允许 `SKILL.md` 与 `references/*.md`

因此，给本仓库制作知识库型 Skill 时应默认：
- 把 `SKILL.md` 写成导航页
- 把主题细节拆到 `references/`
- 不要假设系统会一次性加载全部 reference
- 不要假设任意自定义 frontmatter 字段都会被当前实现消费

推荐测试模式：
- 参照现有知识库 Skill 测试：
  - `internal/skills/store_a2a_knowledge_base_test.go`
  - `internal/skills/store_mcp_knowledge_base_test.go`
  - `internal/skills/store_agent_skills_knowledge_base_test.go`
- 新增一个同风格测试，确认：
  - 索引可见
  - prompt 可读
  - reference 文件存在
