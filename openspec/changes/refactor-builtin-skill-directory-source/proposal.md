# Change: Refactor Builtin Skill Source To Repository Directory

## Why
当前内置 Skill 的真源在 `internal/skills/store.go` 的 `builtinSkills` 大型字符串数组里。启动时再由 `ensureBuiltinSkillsLocked()` 把这些字符串写成运行时 `SKILL.md` 文件。这个模式有几个明显问题：

- 维护成本高：每次修改内置 Skill 都要编辑 Go 代码中的长字符串，而不是直接维护 Skill 目录。
- 结构重复：仓库已经在使用标准 `SKILL.md + references/...` 结构，但内置 Skill 仍绕过这套结构。
- 版本管理差：运行时 `data/skills` 被 `.gitignore` 忽略，不适合作为仓库内置 Skill 的长期真源。
- 风险高：代码内嵌 prompt 与运行时文件内容容易漂移，知识库型 Skill 的 references 也难以统一管理。

用户希望把内置 Skill 的维护方式整改为“新增一个目录，直接维护内置 Skill”，而不是继续通过代码生成 Markdown 文件。

## What Changes
- 新增一个受 Git 管理的内置 Skill 源目录，作为内置 Skill 的唯一真源。
- 内置 Skill 改为从该目录按标准 Skill 结构加载，而不是从 `builtinSkills` 代码常量生成。
- 保持运行时 `SkillsDir` 作为实际工作目录，但其内置 Skill 内容由源目录同步而来。
- 保持 Skill 的启用状态和来源标记独立持久化，不因内容同步而丢失状态。
- 对无效或缺失的内置 Skill 包显式报错，避免静默回退到代码默认值。

## Impact
- Affected specs:
  - `builtin-skill-directory-source`
- Affected code:
  - `internal/skills/store.go`
  - `internal/skills/store_persist.go`
  - `internal/skills/store_core.go`
  - `internal/skills/store_local_api.go`
  - `internal/config/config.go`
  - `cmd/server/main.go`
  - `internal/skills/*test.go`
  - 新增仓库级内置 Skill 源目录及其文件
