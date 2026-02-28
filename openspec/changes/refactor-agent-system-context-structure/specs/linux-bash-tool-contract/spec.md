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
