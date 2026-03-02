## Context
当前 skill 目录已经允许落地 `references/` 与 `scripts/`，但运行时协议只支持把 `SKILL.md` 作为一个整体正文返回。结果是：

- 结构上支持渐进式披露；
- 运行时却无法继续按需读取引用资料；
- 进而逼近“要么全塞进 SKILL.md，要么让模型自己走 bash 读文件”的二选一。

这与本项目“省 token 且稳定”“优先结构化上下文”“不把本地 API 与本地资源读取退回 shell”的长期约束不一致。

## Goals / Non-Goals
- Goals:
  - 让数字分身正式支持 skill 目录下的渐进式文档读取；
  - 保持 `context__read` 的结构化只读边界；
  - 明确 skill 脚本发现与执行的职责分离。
- Non-Goals:
  - 不新增新的本地通用工具；
  - 不把 skill 脚本执行抽象成新的 `skill__run`；
  - 不支持任意文件系统路径读取。

## Decisions
### Decision 1: Extend `skills.read` instead of replacing it with bash
继续保留 `context__read(resource="skills")` 作为唯一标准 skill 读取入口。`skills.read(id)` 继续返回 `SKILL.md`；当附带 `path` 时，读取 skill 目录内白名单文本资源。

原因：
- 保持与现有 `agent-native-api-tools` 一致；
- 继续让读取结果通过结构化工具回传；
- 避免让模型重新依赖 shell 拼接来读 skill 详情。

### Decision 2: Add `skills.index(id)` as the discovery step
新增 `skills.index(id)`，返回该 skill 下的资源索引，至少包含：
- `SKILL.md`
- `references/*.md`
- `scripts/*` 的路径与类型标记

原因：
- 让 references/scripts 都可被发现；
- 保持“先索引，再按需读取/执行”的渐进式披露模式；
- 为后续扩展脚本元数据预留稳定接口。

### Decision 3: Keep script execution on `bash`
skill 脚本不通过 `context__read` 返回正文，不新增执行工具；仍由 `bash` 按声明路径执行。

原因：
- 当前系统已经有稳定执行通道；
- 避免在本次变更中引入新的执行协议；
- 继续保持“读走原生只读工具，执行走执行通道”的职责分离。

## Risks / Trade-offs
- 风险：若 `skills.read(path)` 路径校验不严，可能演变为任意文件读取。
  - 缓解：只允许 skill 根目录下固定白名单路径，拒绝 `..`、绝对路径与非文本 references。
- 风险：模型可能尝试先读取脚本正文。
  - 缓解：runtime prompt 与 skill 注入文案明确“脚本通过 index 发现，通过 bash 执行，不默认正文注入”。
- 风险：旧 skill 不提供 `references/` 或 `scripts/` 时，index 输出为空。
  - 缓解：`skills.index` 仍稳定返回 `SKILL.md` 主入口与空列表，不做静默异常。

## Migration Plan
1. 扩展 skill store 的读取能力与资源索引生成。
2. 增加 `/api/skills/index` 与 `/api/skills/read?id=&path=` 支持。
3. 扩展 `context__read` 对 `skills.index` 与 `skills.read(path)` 的路由。
4. 更新 runtime prompt 与相关 tests。

## Open Questions
- `scripts/*` 是否需要返回建议执行命令模板，还是仅返回相对路径与类型标记。
