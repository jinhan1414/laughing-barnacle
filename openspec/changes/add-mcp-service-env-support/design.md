## Context
当前 MCP 维护能力已经统一到 `maintenance__write -> /api/mcp/services/save -> mcp.Store`，但契约只覆盖 `name/transport/endpoint/command/args/auth_token/enabled`。  
实际运行中，MCP 配置格式本身允许 `env` 出现于不同 transport；像 `tavily-mcp@0.2.3` 这样的 `stdio` 服务还依赖 `TAVILY_API_KEY` 才能工作，因此当前缺口既包含“所有 transport 都无法保存 env”，也包含“stdio 运行时无法消费 env”。

## Goals / Non-Goals
- Goals:
  - 让 MCP 服务可以在产品内保存受控环境变量并实际生效
  - 保持维护链路可回读、可验证，但不泄漏真实 secret
  - 维持当前最小心智负担，不引入新的 secrets 子系统
- Non-Goals:
  - 不引入通用密钥管理器或外部 secret store
  - 不扩展 `context__read(resource="mcp")` 的新动作类型
  - 不在本次提案中引入基于 `env` 的 HTTP header 模板、URL 模板或额外变量替换语法

## Decisions
### Decision 1: 所有 MCP transport 都接受并保存 `env`
用户要求 MCP 配置能力与通用 MCP 服务定义保持一致，因此本次不再限制为 `stdio` 专属字段：
- `streamable_http` 可保存 `env`
- `sse` 可保存 `env`
- `stdio` 可保存 `env`

这样可以先补齐配置契约，避免维护接口与常见 MCP 配置格式继续脱节。

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

### Decision 4: 运行时按 transport 分层消费 `env`
当前只有 `stdio` transport 直接启动本地子进程，因此具备明确的 `env` 消费点。  
本次运行时语义拆分为：
- 所有 transport：保存、持久化、脱敏回读时都接受 `env`
- `stdio`：启动子进程时将 `env` 合并进进程环境
- `streamable_http` / `sse`：当前版本先保证配置不被拒绝、不被丢失，后续若要消费这些变量，再通过独立提案定义具体语义

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
- 风险：用户可能预期 HTTP/SSE 当前也会立即消费 `env`。
  - 缓解：在设计与任务中显式声明，本次先补齐“可保存、可回读、不拒绝”的契约；只有 `stdio` 增加明确运行时注入。
- 风险：`env` 更新语义若未说明清楚，普通用户可能误以为空白会清空。
  - 缓解：设置页文案与 Skill 文案明确“留空不变，`{}` 清空”。
- 风险：如果只补接口不补运行时，仍然无法真正使用。
  - 缓解：本提案把持久化、回读与 `stdio` 运行时注入放在同一 change 中交付。

## Migration Plan
1. 扩展 MCP service 数据模型、持久化与校验逻辑。
2. 扩展 `/api/mcp/services/save` 与 `/settings/mcp/save` 的 `env` 输入契约，覆盖所有 transport。
3. 扩展 `/api/mcp/services` 的脱敏返回结构。
4. 在 `client_stdio.go` 注入服务级环境变量。
5. 更新 MCP 设置页、维护 Skill 与相关测试。
