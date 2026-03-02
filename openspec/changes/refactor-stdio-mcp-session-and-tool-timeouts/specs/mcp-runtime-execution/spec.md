## ADDED Requirements
### Requirement: Stdio MCP Sessions Must Be Reused Per Service
系统 MUST 按 `service.id` 复用已初始化的 `stdio` MCP 子进程会话，避免每次 RPC 都重新启动并握手。

#### Scenario: Reuse initialized stdio session for multiple RPCs
- **WHEN** 同一个已启用 `stdio` MCP 服务先后执行 `tools/list` 与 `tools/call`
- **THEN** 系统 MUST 复用同一个已初始化子进程完成两次 RPC
- **AND** 不得在第二次 RPC 前重新执行一次新的进程启动与 `initialize`

### Requirement: Stdio Session Must Be Recreated After Invalidation
系统 MUST 在 `stdio` 会话超时、读写失败、进程退出或服务配置签名变化后销毁旧会话，并在下次调用时重建。

#### Scenario: Recreate stdio session after timeout
- **WHEN** 某次 `stdio` MCP 调用因请求上下文超时而失败
- **THEN** 系统 MUST 销毁该服务对应的旧子进程会话
- **AND** 后续对同一服务的新调用 MUST 使用新建会话重新初始化

#### Scenario: Recreate stdio session after config change
- **WHEN** 同一 `service.id` 的 `command`、`args` 或 `env` 发生变化
- **THEN** 系统 MUST 丢弃旧会话
- **AND** 后续调用 MUST 基于最新配置创建新会话
