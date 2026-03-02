## ADDED Requirements
### Requirement: Skill Scripts Execute Through bash
系统 MUST 将 skill 打包脚本的执行保留在 `bash` 通道内，并与 skill 文档读取分离。

#### Scenario: Execute declared skill script via bash
- **WHEN** 模型已通过 skill 索引或 skill 正文确认某个 `scripts/*` 入口需要执行
- **THEN** 系统可通过 `bash` 执行该脚本
- **AND** 不要求将脚本正文先注入上下文

#### Scenario: Prefer native skill reads over shell file reads
- **WHEN** 模型需要读取 skill 的说明文档或 `references/*.md`
- **THEN** 优先使用 `context__read(resource="skills", ...)`
- **AND** 不将 `bash` 目录扫描、`cat` 或 `curl /api/skills/read` 作为默认读取路径
