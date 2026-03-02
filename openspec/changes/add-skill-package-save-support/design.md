## Context
当前 Skill 读取链路已经支持：

- `context__read(resource="skills", action="index", id="<skill_id>")`
- `context__read(resource="skills", action="read", id="<skill_id>", path="references/<file>.md")`

同时，Store 已能从目录结构中发现 `references/` 与 `scripts/`。但写链路仍停留在单个 `SKILL.md`，造成运行时能力与维护契约不一致。

## Goals / Non-Goals
- Goals:
  - 允许数字分身通过一次结构化写入创建完整 Skill 包
  - 维持现有 `prompt -> SKILL.md` 主体渲染方式
  - 严格限制可写路径，避免 Skill 目录成为任意文件写入口
- Non-Goals:
  - 不改变 `context__read(resource="skills")` 的读取协议
  - 不新增新的通用文件写工具
  - 不修改 skills.sh 安装链路

## Decisions
### Decision 1: Extend `skills.save` with a structured `resources` array
`skills.save` 新增可选字段：

```json
{
  "resources": [
    {"path":"references/guide.md","content":"# Guide"},
    {"path":"scripts/check.ps1","content":"Write-Output 'ok'"}
  ]
}
```

原因：
- 结构清晰，便于 schema 校验
- token 开销低于整棵树字符串
- 可与现有 `prompt` 主体解耦

### Decision 2: Keep `SKILL.md` as rendered output from metadata plus prompt
主文件仍由 `id/name/description/prompt` 渲染，不允许调用方通过 `resources` 直接覆盖 `SKILL.md`。

原因：
- 保持现有存储格式稳定
- 避免主文件来源分裂

### Decision 3: Restrict writable paths to `references/*.md` and `scripts/*`
服务端仅允许写入：

- `references/<file>.md`
- `scripts/<file>`

禁止绝对路径、`..`、空段与其他顶级目录。

原因：
- 防止路径穿越与意外污染 Skill 根目录
- 与现有读取白名单保持一致

### Decision 4: Replace the target skill directory atomically
更新 Skill 时，先清理目标目录，再重建 `SKILL.md` 与资源文件，作为单次 package 落盘。

原因：
- 避免旧资源残留
- 保持“当前 payload 即完整真相”的维护语义

## Risks / Trade-offs
- 风险：更新 Skill 时未声明的旧资源会被删除。
  - 缓解：将 `skills.save` 定义为完整包覆盖语义，并在维护 Skill 文案中明确。
- 风险：脚本类型较多，若限制扩展名过严会误伤合法脚本。
  - 缓解：`scripts/` 允许任意文件名，但仍校验路径安全。

## Migration Plan
1. 扩展 API 请求结构与 tool payload 校验。
2. 在 Store 增加 package 写入能力与路径校验。
3. 更新维护 Skill 文案。
4. 补测试并回归 `skills.read/index` 读取链路。
