## 1. Specification
- [x] 1.1 新增 `builtin-skill-directory-source` 规范，定义内置 Skill 的目录真源、同步边界与显式失败语义。
- [x] 1.2 通过 `openspec validate refactor-builtin-skill-directory-source --strict`。

## 2. Builtin Skill Source Refactor
- [x] 2.1 新增仓库级内置 Skill 源目录，并将现有代码内嵌 builtin Skill 迁移为标准 Skill 包结构。
- [x] 2.2 为配置层与启动层增加内置 Skill 源目录接线，保持运行时 `SkillsDir` 不变。
- [x] 2.3 重构 `internal/skills` 的 builtin 加载逻辑：从目录读取、校验并同步到运行时目录，替代 `builtinSkills` 内容常量。
- [x] 2.4 保持 `skills_state.json` 的启停状态和 `source=builtin` 语义，明确 builtin 覆盖同步规则。

## 3. Verification
- [x] 3.1 单测：Store 启动时可从内置 Skill 源目录同步 `SKILL.md` 与 references/scripts。
- [x] 3.2 单测：无效 builtin Skill 包会导致显式初始化错误。
- [x] 3.3 单测：builtin Skill 内容刷新不会丢失启停状态。
- [x] 3.4 `go test ./internal/skills -timeout 60s -count=1`
- [x] 3.5 `go test ./internal/agent ./internal/web -run "Skill|skills" -timeout 60s -count=1`
