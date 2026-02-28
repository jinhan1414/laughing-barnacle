## ADDED Requirements
### Requirement: Native Maintenance Write Tool as Default
系统 MUST 提供并优先使用原生维护写入工具承接 MCP/Skills/Schedules/A2A 的 JSON 写入请求。

#### Scenario: Route maintenance writes through native tool
- **WHEN** 模型需要执行维护写入（save|toggle|delete|run|install）
- **THEN** 默认调用 `maintenance__write`
- **AND** 由宿主完成 HTTP 请求与 JSON 序列化

#### Scenario: Reject out-of-contract maintenance requests
- **WHEN** `maintenance__write` 收到未在白名单中的资源类型或操作类型
- **THEN** 系统返回显式参数错误
- **AND** 不发起任何维护写入请求

### Requirement: No Shell Compatibility Path for Maintenance Writes
系统 MUST 移除维护写入的 shell 兼容路径，不再允许通过 `linux__bash` 拼接命令执行本地 API 写入。

#### Scenario: Runtime prompt prohibits shell-based maintenance writes
- **WHEN** 系统注入维护写入约束
- **THEN** 文案明确维护写入只能走 `maintenance__write`
- **AND** 不出现任何 shell 命令作为本地 API 写入路径的说明
