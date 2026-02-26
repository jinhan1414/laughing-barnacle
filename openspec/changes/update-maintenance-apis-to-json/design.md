## Context
当前维护能力存在两条写链路：
- LLM/Skill：偏向 `linux__bash + curl + 表单`
- 人工设置页：`/settings/*` 表单

在 shell 与提示词存在不确定性的前提下，表单拼接比 JSON 更脆弱。A2A 已切换 JSON 并验证稳定，需把维护链路统一到同一协议形态，降低执行偏差与调试成本。

## Goals / Non-Goals
- Goals:
  - 维护类写操作统一 JSON 协议与结构化参数校验
  - 维护类 Skill 指令模板统一 JSON 示例
  - 保持设置页可用，不中断人工运维
- Non-Goals:
  - 不改动业务语义（仅改调用协议与入口）
  - 不引入新的 fallback 或 mock 成功路径
  - 不修改 A2A 已经稳定的 JSON 方案

## Decisions
### Decision 1: 新增统一 JSON 维护 API
新增并对齐以下写入端点（POST + `application/json`）：
- MCP：`/api/mcp/services/save|toggle|delete`
- Skills：`/api/skills/save|toggle|delete|install`
- Schedules：`/api/schedules/save|toggle|delete|run`

每个端点使用明确请求结构体与字段校验，错误返回 JSON，不再依赖重定向 query 参数承载错误语义。

### Decision 2: 表单端点保留，但下沉为兼容层
`/settings/*` 表单端点继续保留给设置页使用；  
内部统一转换到相同 service 写入逻辑，避免 API/页面两套规则分叉。

### Decision 3: 维护 Skill 全量切换 JSON 指令模板
以下 Skill 改为 JSON 维护模板：
- `mcp-config-maintainer`
- `skills-config-maintainer`
- `schedule-config-maintainer`

要求包含：
- JSON 写入示例
- 写后回读验证步骤
- 失败显式暴露约束

### Decision 4: 运行时提示统一 JSON 约束
`tool_runtime_prompt` 同步改为“维护接口默认 JSON”，避免模型继续生成表单写法。  
在 Windows PowerShell 下继续强调 `curl.exe` 与 `Invoke-RestMethod` 优先级。

### Decision 5: 分阶段迁移与回归验证
实施分两段：
1. 先补 JSON API + 测试 + Skill 文案切换
2. 再将设置页表单处理重构为共享 service 层

每段都必须通过 `go test ./... -timeout 60s` 与 OpenSpec 严格校验。

## Risks / Trade-offs
- 风险：接口数量短期增加（新旧并存）。  
  缓解：保持单一 service 写入逻辑，避免行为分叉。
- 风险：已有自动化脚本仍调用 `/settings/*`。  
  缓解：兼容层保留，不做破坏式下线。
- 风险：Skill 文案切换后旧提示残留。  
  缓解：同步更新内置 Skill 模板与 `data/skills` 落地文件，并补注入测试。

## Migration Plan
1. 定义 JSON 请求结构与路由注册。
2. 为 MCP/Skills/Schedules 增加 JSON 维护处理器。
3. 抽取共享写入 service，供 `/api/*` 与 `/settings/*` 共用。
4. 更新维护类 Skill 模板到 JSON。
5. 更新 runtime prompt 的维护约束文案。
6. 增加并通过单测/集成测试。
