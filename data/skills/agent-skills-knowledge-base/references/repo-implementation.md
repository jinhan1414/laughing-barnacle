# 本仓库中的 Skill 现状

本仓库当前如何存储 Skill：
- `internal/skills/store_persist.go`
  - 启动时扫描 `skills` 目录下每个子目录的 `SKILL.md`。
- `internal/skills/store_markdown.go`
  - 当前解析层稳定读取 `SKILL.md` frontmatter 中的 `name`、`description` 和正文。
- `internal/skills/store_core.go`
  - 提供 `ListEnabledSkillIndex`、`ReadEnabledSkillPrompt`、保存/删除/启停等能力。

本仓库如何做渐进式披露：
- `internal/agent/reply_skill_index.go`
  - 首轮先注入 Skills 索引，不默认注入所有 Skill 正文。
- `internal/agent/context_read_tool.go`
  - 运行时通过 `context__read(resource="skills", action="list|index|read")` 按需读取 Skill。
- `internal/skills/store_resources.go`
  - Skill 资源索引只暴露：
    - `SKILL.md`
    - `references/*.md`
    - `scripts/*`
- `internal/web/server_api_skill_resources.go`
  - 对外提供 `/api/skills/index` 与 `/api/skills/read`。

本仓库当前 Skill 目录口径：
- 代码实现按本地文件夹模式工作：
  - `data/skills/<skill_id>/SKILL.md`
  - `data/skills/<skill_id>/references/*.md`
  - `data/skills/<skill_id>/scripts/*`
- 这是产品当前实现，不等于开放规范所有可选能力都已支持。

维护与写入：
- 维护接口在 `/api/skills` 相关端点上。
- Agent 侧还有 `maintenance__write(resource="skills", action="save|toggle|delete|install")` 这类结构化维护入口。

回答“本仓库支持哪些 Skill 能力”时，建议这样说：
- 已支持：
  - Skill 目录扫描
  - `SKILL.md` 解析
  - 索引注入
  - 资源索引
  - 按需读取
  - 启停与保存
- 不应默认声称已完整支持：
  - 开放 Agent Skills 规范中的全部元数据字段
  - 所有跨平台兼容声明
  - 任意脚本/资源类型的自动理解与执行
