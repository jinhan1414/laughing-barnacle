## ADDED Requirements
### Requirement: Skills Save Supports Structured Skill Packages
系统 MUST 允许 `POST /api/skills/save` 及对应维护工具在保存 Skill 时携带结构化资源列表，以一次性写入完整 Skill 包。

#### Scenario: Save skill package with references and scripts
- **WHEN** 调用 `POST /api/skills/save`，请求体包含合法的 `id/name/description/prompt/enabled` 以及 `resources[]`
- **THEN** 系统写入 `SKILL.md` 与声明的资源文件
- **AND** 后续 `skills.index` 可以返回这些资源
- **AND** 后续 `skills.read` 可以读取允许暴露的引用文档

#### Scenario: Reject invalid skill resource path on save
- **WHEN** 调用 `POST /api/skills/save`，`resources[].path` 包含非法路径、路径穿越或未授权目录
- **THEN** 系统返回明确错误状态与错误消息
- **AND** 不写入任何 Skill 内容
