## ADDED Requirements
### Requirement: Builtin Skills Must Load From A Dedicated Repository Directory
系统 MUST 使用受版本控制的专用目录作为内置 Skill 的内容真源，而不是将内置 Skill 正文硬编码在 Go 代码中。

#### Scenario: Sync builtin skill package from repository source
- **WHEN** Store 启动并扫描内置 Skill 源目录
- **THEN** 系统从 `<builtin_skill_dir>/<skill_id>/SKILL.md` 读取内置 Skill 内容
- **AND** 将该 Skill 以标准目录结构同步到运行时 `SkillsDir`

### Requirement: Builtin Skill Packages Must Use Standard Skill Layout
系统 MUST 以标准 Skill 包结构维护内置 Skill，并同步其可读资源与脚本目录。

#### Scenario: Preserve references and scripts for builtin skill
- **WHEN** 内置 Skill 源目录下存在 `references/` 或 `scripts/`
- **THEN** 系统同步这些资源到运行时 Skill 目录
- **AND** 运行时 Skill 资源索引仍可发现对应文件

### Requirement: Builtin Skill Sync Must Preserve Runtime State
系统 MUST 在刷新内置 Skill 内容时保留运行时状态文件中的启停状态与来源标记。

#### Scenario: Refresh builtin content without losing enabled state
- **WHEN** 某个 `source=builtin` 的 Skill 在状态文件中已被显式启停，且启动时发生内置内容同步
- **THEN** 该 Skill 的正文与资源更新为源目录内容
- **AND** 其 `enabled` 状态保持不变

### Requirement: Invalid Builtin Skill Packages Must Fail Explicitly
系统 MUST 在内置 Skill 源目录缺失必要文件或结构非法时显式失败，不得静默回退到代码默认值。

#### Scenario: Reject malformed builtin skill package on startup
- **WHEN** 内置 Skill 源目录中的某个 Skill 缺失 `SKILL.md` 或无法解析最小 Skill 结构
- **THEN** Store 初始化失败并返回明确错误
- **AND** 系统不继续使用静态代码内嵌 prompt 作为后备来源
