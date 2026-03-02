## Context
当前 MCP 维护能力已经统一到 `maintenance__write -> /api/mcp/services/save -> mcp.Store`，但契约只覆盖 `name/transport/endpoint/command/args/auth_token/enabled`。  
实际运行中：
- `stdio` transport 依赖进程环境变量
- HTTP 类 transport 依赖请求头，其中现有代码只覆盖了 `Authorization`

因此当前缺口不是“所有 transport 统一缺同一种字段”，而是“不同 transport 都缺少各自应有的配置入口”。

## Goals / Non-Goals
- Goals:
  - 让 MCP 服务可以按 transport 语义在产品内保存定向配置并实际生效
  - 保持维护链路可回读、可验证，但不泄漏真实 secret
  - 维持当前最小心智负担，不引入新的 secrets 子系统
- Non-Goals:
  - 不引入通用密钥管理器或外部 secret store
  - 不扩展 `context__read(resource="mcp")` 的新动作类型
  - 不引入 header 模板、变量替换语法或按请求动态改写逻辑

## Decisions
### Decision 1: 按 transport 拆分配置字段
本次按 transport 语义拆分，而不是给所有 transport 强塞同一字段：
- `stdio`：支持 `env`
- `streamable_http`：支持 `headers`
- `sse`：支持 `headers`

这样与 MCP 客户端实际消费点一致，也避免出现“字段可保存但没有明确运行时语义”的半成品能力。

### Decision 2: 更新语义采用“省略保留，空对象清空”
当前 `auth_token` 已有“留空表示不变”的更新语义。  
`env` / `headers` 都采用同一思路，但保留显式清空能力：
- 保存 payload 省略 `env` / `headers`：保留已有值
- 保存 payload 传 `env: {}` / `headers: {}`：清空已有值
- 保存 payload 传对象：整体替换为新集合

这比额外引入 `clear_env` 布尔位更省字段、更稳定。

### Decision 3: 回读只暴露元数据，不回显真实值
`env` 与 `headers` 都可能承载 secret。  
因此 `/api/mcp/services` 与 `context__read(resource="mcp", action="list")` 只返回：
- `has_env`
- `env_keys`
- `has_headers`
- `header_keys`

不返回真实值，也不在设置页列表中展示 value。这样仍然能完成“是否已配置、配置了哪些键”的回读校验。

### Decision 4: 运行时按 transport 消费对应字段
启动 `stdio` MCP 子进程时，运行时环境采用：
- 基础：宿主进程当前环境
- 覆盖：服务配置中的 `env`

HTTP 类 MCP 发起请求时，运行时请求头采用：
- 基础：协议必需头（如 `Content-Type`、`Accept`、`MCP-Protocol-Version`）
- 认证：现有 `Authorization` 逻辑
- 追加：服务配置中的 `headers`

若同名键冲突：
- 协议必需头不可被自定义配置覆盖
- `Authorization` 继续由现有认证字段负责
- 其他头允许由 `headers` 提供

### Decision 5: 自定义 header 只承接非 Authorization 信息
已有 `auth_token` 已稳定映射到 `Authorization: Bearer <token>`。  
因此新增 `headers` 时明确禁止用户再通过它写 `Authorization`，避免出现双源冲突。

### Decision 6: 设置页维持最小可编辑形态
设置页 MCP 表单补两个 JSON 输入区即可：
- `stdio` 用 `env`
- HTTP 类 transport 用 `headers`

交互语义与接口保持一致：
- 留空表示不变
- 输入 `{}` 表示清空
- 列表仅展示 `env_keys` / `header_keys`

## Risks / Trade-offs
- 风险：secret 仍会持久化在本地 settings 文件。
  - 缓解：本次只做脱敏回读，不把 value 暴露到列表/API 回读；后续若要加强，再单独提 secret 管理提案。
- 风险：自定义 `headers` 可能与协议头或认证头冲突。
  - 缓解：明确禁止覆盖协议必需头和 `Authorization`。
- 风险：`env` 更新语义若未说明清楚，普通用户可能误以为空白会清空。
  - 缓解：设置页文案与 Skill 文案明确“留空不变，`{}` 清空”；`headers` 同理。
- 风险：如果只补接口不补运行时，仍然无法真正使用。
  - 缓解：本提案把持久化、回读、`stdio` 环境注入与 HTTP 头注入放在同一 change 中交付。

## Migration Plan
1. 扩展 MCP service 数据模型、持久化与校验逻辑。
2. 扩展 `/api/mcp/services/save` 与 `/settings/mcp/save` 的 `env` / `headers` 输入契约，并按 transport 校验。
3. 扩展 `/api/mcp/services` 的脱敏返回结构。
4. 在 `client_stdio.go` 注入服务级环境变量。
5. 在 `client_session_http.go` 注入服务级自定义请求头。
5. 更新 MCP 设置页、维护 Skill 与相关测试。
