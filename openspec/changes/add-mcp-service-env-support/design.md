## Context
当前 MCP 维护能力已经统一到 `maintenance__write -> /api/mcp/services/save -> mcp.Store`，但契约只覆盖 `name/transport/endpoint/command/args/auth_token/enabled`。  
实际运行中，像 `tavily-mcp@0.2.3` 这样的 `stdio` 服务需要 `TAVILY_API_KEY` 才能工作，因此“不能保存 env”并不只是 UI/接口缺字段，而是整条配置与运行链路缺失。

## Goals / Non-Goals
- Goals:
  - 让 MCP 服务可以在产品内保存受控环境变量并实际生效
  - 保持维护链路可回读、可验证，但不泄漏真实 secret
  - 维持当前最小心智负担，不引入新的 secrets 子系统
- Non-Goals:
  - 不引入通用密钥管理器或外部 secret store
  - 不扩展 `context__read(resource="mcp")` 的新动作类型
  - 不为 HTTP/SSE MCP 追加无实际用途的 `env` 运行时语义

## Decisions
### Decision 1: `env` 仅支持 `stdio` MCP
问题源头是 `stdio` 子进程缺少环境变量。  
因此本次能力只覆盖 `transport=stdio` 的 MCP 服务：
- `stdio` 可保存 `env`
- `streamable_http` / `sse` 若传入 `env`，显式报错

这样可以把实现收紧到真实需求，避免先把 schema 放宽、再留下“保存了但根本不会用”的半成品能力。

### Decision 2: 更新语义采用“省略保留，空对象清空”
当前 `auth_token` 已有“留空表示不变”的更新语义。  
`env` 采用同一思路，但保留显式清空能力：
- 保存 payload 省略 `env`：保留已有 `env`
- 保存 payload 传 `env: {}`：清空已有 `env`
- 保存 payload 传 `env: {"KEY":"VALUE"}`：整体替换为新集合

这比额外引入 `clear_env` 布尔位更省字段、更稳定。

### Decision 3: 回读只暴露元数据，不回显真实值
`env` 里会存 API Key。  
因此 `/api/mcp/services` 与 `context__read(resource="mcp", action="list")` 只返回：
- `has_env`
- `env_keys`

不返回真实值，也不在设置页列表中展示 value。这样仍然能完成“是否已配置、配置了哪些键”的回读校验。

### Decision 4: `stdio` 运行时合并宿主环境与服务环境
启动 `stdio` MCP 子进程时，运行时环境采用：
- 基础：宿主进程当前环境
- 覆盖：服务配置中的 `env`

若同名键冲突，服务配置值覆盖宿主值。这样既保留现有 PATH 等基础环境，也能让服务级配置真正生效。

### Decision 5: 设置页维持最小可编辑形态
设置页 MCP 表单补一个 `env` JSON 输入区即可，不新建复杂 secret 管理 UI。  
交互语义与接口保持一致：
- 留空表示不变
- 输入 `{}` 表示清空
- 列表仅展示 `env_keys`

## Risks / Trade-offs
- 风险：secret 仍会持久化在本地 settings 文件。
  - 缓解：本次只做脱敏回读，不把 value 暴露到列表/API 回读；后续若要加强，再单独提 secret 管理提案。
- 风险：`env` 更新语义若未说明清楚，普通用户可能误以为空白会清空。
  - 缓解：设置页文案与 Skill 文案明确“留空不变，`{}` 清空”。
- 风险：如果只补接口不补运行时，仍然无法真正使用。
  - 缓解：本提案把持久化、回读、运行时注入放在同一 change 中交付。

## Migration Plan
1. 扩展 MCP service 数据模型、持久化与校验逻辑。
2. 扩展 `/api/mcp/services/save` 与 `/settings/mcp/save` 的 `env` 输入契约。
3. 扩展 `/api/mcp/services` 的脱敏返回结构。
4. 在 `client_stdio.go` 注入服务级环境变量。
5. 更新 MCP 设置页、维护 Skill 与相关测试。
