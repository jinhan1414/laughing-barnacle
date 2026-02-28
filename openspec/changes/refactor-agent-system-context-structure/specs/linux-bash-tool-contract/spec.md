## ADDED Requirements
### Requirement: Environment Disambiguation for linux__bash
系统 MUST 在运行时提示中显式声明 `linux__bash` 的“工具名”与“真实执行 shell”关系，避免模型按名称误推断语法。

#### Scenario: Windows runtime explicitly warns PowerShell semantics
- **WHEN** 当前可用 shell 为 Windows PowerShell/pwsh
- **THEN** 运行时提示明确说明 `linux__bash` 实际在 PowerShell 执行
- **AND** 同时约束命令语法遵循 PowerShell 语义（含 `curl.exe` 规则）

#### Scenario: Linux runtime keeps shell-specific semantics
- **WHEN** 当前可用 shell 为 bash/sh
- **THEN** 运行时提示明确当前 shell 为 Linux
- **AND** 不混入 Windows 特定语法约束

### Requirement: linux__bash Scope Is Shell Execution
系统 MUST 将 `linux__bash` 定位为 shell 命令执行工具，不作为本地 API 维护读写的默认入口。

#### Scenario: Keep linux__bash for non-API shell tasks
- **WHEN** 模型需要执行文件系统、进程、时间等本地 shell 操作
- **THEN** 系统允许调用 `linux__bash`
- **AND** 不要求切换到 API 原生工具

#### Scenario: Local API interaction defaults to dedicated tools
- **WHEN** 模型需要读取索引详情或执行维护写入
- **THEN** 系统默认引导调用对应原生工具
- **AND** 不把 `linux__bash + curl` 作为默认首选链路
