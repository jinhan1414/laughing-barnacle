## Context
当前内置 Skill 的生命周期是：

1. 在 `internal/skills/store.go` 中硬编码 `builtinSkills []Skill`
2. `Store.load()` 调用 `ensureBuiltinSkillsLocked()`
3. 启动时把代码中的 prompt 写到运行时 `SkillsDir/<skill_id>/SKILL.md`

这使内置 Skill 的维护方式与项目已经采用的标准 Skill 目录结构脱节。尤其是知识库型 Skill 已经开始依赖 `references/*.md`，继续把内置 Skill 真源放在代码里，会导致：

- 代码文件过大且难审阅
- references/scripts/assets 难以作为完整包维护
- `data/skills` 作为运行时目录又被 `.gitignore` 忽略，不适合作为仓库真源

因此，本次变更需要把“内置 Skill 真源”从 Go 代码迁移到一个专门的、受版本控制的目录。

## Goals / Non-Goals
- Goals:
  - 让内置 Skill 以标准目录结构在仓库中维护
  - 让运行时继续从 `SkillsDir` 读取 Skill，不破坏现有注入和 API 逻辑
  - 保持内置 Skill 的启停状态与内容同步职责分离
  - 让无效内置 Skill 包在启动时显式失败
- Non-Goals:
  - 不改变 Skill 注入策略或 `context__read(resource="skills")` 协议
  - 不改变 skills.sh 安装和手动 Skill 保存逻辑
  - 不在本次提案中重做 builtin skill 的删除/覆盖策略

## Decisions
### Decision 1: Introduce a dedicated repository-tracked builtin skill directory
新增独立目录，例如 `builtin-skills/`，作为内置 Skill 的唯一真源。

原因：
- 该目录受 Git 管理，适合长期维护内置内容
- 避免继续把内置 Skill 混在 `.gitignore` 的 `data/` 下
- 直接复用标准 Skill 包结构：`<skill_id>/SKILL.md`、`references/`、`scripts/`

### Decision 2: Keep runtime `SkillsDir` as the effective read/write store
运行时仍继续使用现有 `SkillsDir` 作为 Skill Store 的实际目录；内置 Skill 只是在启动时从 `builtin-skills/` 同步到 `SkillsDir`。

原因：
- 不破坏现有 `/api/skills*`、设置页、索引注入、资源读取逻辑
- 不需要重写外部 Skill 的保存与安装路径
- 将“内置真源”和“运行时工作目录”职责分离

### Decision 3: Replace code literals with file-backed builtin package loading
移除 `builtinSkills []Skill` 作为内容真源，改为扫描 `builtin-skills/<skill_id>/SKILL.md` 并加载完整包。

原因：
- Skill 内容回到 Skill 文件自身维护
- references/scripts 可以一起版本化
- 内置 Skill 与普通 Skill 统一为同一种目录形态

### Decision 4: Preserve state, but refresh builtin-managed content deterministically
`skills_state.json` 继续记录 `enabled` 与 `source=builtin`。当某个 Skill 源自内置目录时，启动同步应覆盖其运行时内容，但保留状态文件中的启停状态。

原因：
- 保持当前启停语义
- 防止运行时目录中的旧内容漂移
- 让“内容来自仓库目录，状态来自运行时状态文件”边界清晰

### Decision 5: Fail explicitly on malformed builtin packages
若 `builtin-skills/<skill_id>/SKILL.md` 缺失、frontmatter 非法或目录结构不满足最小要求，Store 初始化必须显式失败。

原因：
- 符合当前项目“暴露失败、不做静默 fallback”的规则
- 避免错误内置 Skill 被部分加载后污染运行时

## Risks / Trade-offs
- 风险：引入新的目录配置，启动路径错误会导致 Store 初始化失败。
  - 缓解：提供明确默认值与显式错误信息。
- 风险：内置 Skill 同步到运行时目录时，可能与用户手动修改的同名目录冲突。
  - 缓解：仅对 `source=builtin` 或缺失记录的内置目标执行覆盖同步；将覆盖策略在实现与测试中写清楚。
- 风险：当前部分测试依赖代码内置 prompt，迁移后测试需要改为文件夹 fixture。
  - 缓解：在 tasks 中明确补齐对应迁移测试。

## Migration Plan
1. 引入新的内置 Skill 源目录与默认配置项。
2. 将现有代码内嵌的内置 Skill 迁移为目录文件。
3. 将 Store 的 builtin 加载逻辑改为“目录读取 + 运行时同步”。
4. 更新所有依赖 `builtinSkills` 的单测与启动接线。
5. 验证启停状态、资源索引、知识库 Skill references 等行为不回退。

## Open Questions
- builtin 源目录是否需要单独配置项（例如 `APP_BUILTIN_SKILLS_DIR`），还是固定为仓库相对路径；本提案默认采用可配置目录，避免路径硬编码到 Store。
