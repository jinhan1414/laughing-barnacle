# Change: 统一维护接口与维护 Skill 为 JSON 调用

## Why
当前除 A2A 之外的维护链路（MCP、Skills、Schedules）仍主要依赖 `/settings/*` 表单写入。  
在真实执行中，这种 `--data-urlencode` 方案在 Windows shell 下更容易出现引号、转义和字段拼装错误，导致模型消耗多轮工具调用仍无法闭环。  
已落地的 A2A JSON 维护链路验证了“结构化请求 + 明确校验 + 回读确认”的稳定性，需推广到其余维护能力。

## What Changes
- 新增 MCP、Skills、Schedules 的 JSON 维护接口（`/api/*`）：
  - MCP：`/api/mcp/services/save|toggle|delete`
  - Skills：`/api/skills/save|toggle|delete|install`
  - Schedules：`/api/schedules/save|toggle|delete|run`
- 将维护类 Skill 全部切换为 JSON 写入规范，不再要求 `--data-urlencode`。
- 更新工具运行时约束提示，统一维护链路默认走 JSON 接口。
- 保留设置页表单入口用于人工操作，但后端写入逻辑收敛为同一服务层，避免双份规则漂移。
- 为 JSON 维护链路补充参数校验、错误透传、回读校验与回归测试。

## Impact
- Affected specs: `maintenance-json-interfaces`
- Affected code:
  - `internal/web/server_settings_mcp_skill.go`
  - `internal/web/server_settings_schedule_memory_llm.go`
  - `internal/web/server_setup.go`
  - `internal/agent/tool_runtime_prompt.go`
  - `internal/skills/store.go`
  - `data/skills/*-config-maintainer/SKILL.md`
  - `internal/web/*test.go`、`internal/skills/*test.go`
