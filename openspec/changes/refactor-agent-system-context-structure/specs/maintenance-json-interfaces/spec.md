## ADDED Requirements
### Requirement: PowerShell-First JSON Write Guidance
系统 MUST 在 Windows PowerShell 场景下优先引导维护写入使用 `Invoke-RestMethod + ConvertTo-Json`，避免默认手写复杂 JSON 转义命令。

#### Scenario: Runtime prompt prefers host-side JSON construction
- **WHEN** 系统注入维护接口写入指导且运行环境为 PowerShell
- **THEN** 提示文案优先给出 `Invoke-RestMethod + ConvertTo-Json` 路径
- **AND** 仍要求 `Content-Type: application/json`

#### Scenario: Curl path remains explicit but secondary
- **WHEN** 提示中包含 curl 写入示例
- **THEN** 文案明确使用 `curl.exe` 且保留 JSON body 约束
- **AND** 不把手写多层转义作为默认首选路径

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
