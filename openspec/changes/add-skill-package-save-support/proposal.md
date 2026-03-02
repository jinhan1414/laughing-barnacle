# Change: Add Skill Package Save Support

## Why
当前系统运行时已经支持 Skill 目录中的 `references/` 和 `scripts/` 资源被索引与按需读取，但手动创建/更新 Skill 的写入接口仍只接受 `id/name/description/prompt/enabled`。这导致数字分身只能保存单个 `SKILL.md`，无法创建完整的 Skill 包。

继续把引用资料或脚本塞回单个 `prompt` 会破坏现有“渐进式披露”约束，并显著增加上下文开销。因此需要把 `skills.save` 从“保存 skill 元数据”升级为“保存 skill 包”。

## What Changes
- 扩展 `POST /api/skills/save` 与 `maintenance__write(resource="skills", action="save")` 的请求契约，允许携带结构化 `resources` 数组。
- Skill 持久化层新增完整包写入能力：一次性落盘 `SKILL.md`、`references/*.md`、`scripts/*`。
- 对 Skill 资源路径增加白名单校验，拒绝路径穿越与非约定目录写入。
- 更新维护类 Skill 文案，使数字分身知道如何用结构化 payload 创建带 resources 的 Skill。
- 补充回归测试，覆盖 API、存储与资源回读链路。

## Impact
- Affected specs:
  - `maintenance-json-interfaces`
- Affected code:
  - `internal/agent/maintenance_write_tool.go`
  - `internal/skills/store.go`
  - `internal/skills/store_core.go`
  - `internal/web/server_api_maintenance_manage.go`
  - `internal/web/server_settings_mcp_skill.go`
  - `internal/web/server_maintenance_service.go`
  - `builtin-skills/skills-config-maintainer/SKILL.md`
  - `internal/skills/*test.go`
  - `internal/web/*test.go`
