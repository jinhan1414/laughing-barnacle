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
