# linux-bash-tool-contract Specification

## Purpose
TBD - created by archiving change update-linux-bash-command-only-input. Update Purpose after archive.
## Requirements
### Requirement: Command-Only bash Arguments
系统 MUST 将 `bash` 的调用参数定义为单个完整命令字符串，不再要求对象键包装。

#### Scenario: Execute by direct command string
- **WHEN** 模型调用 `bash` 并提供非空命令字符串（如 `curl -sS http://127.0.0.1:9080/api/a2a/agents`）
- **THEN** 系统按该命令执行并返回标准执行结果（`exit_code/shell/stdout/stderr`）
- **AND** 不要求 `command` 键

### Requirement: Explicit Rejection for Legacy Object Arguments
系统 MUST 显式拒绝旧的对象参数格式（如 `{"command":"..."}`），禁止静默兼容或自动转换。

#### Scenario: Legacy object payload is rejected
- **WHEN** `bash` 收到对象参数或其他非字符串参数
- **THEN** 系统返回明确参数错误
- **AND** 不执行任何 shell 命令

### Requirement: Runtime Guidance Consistency for Command-Only Contract
系统 MUST 在运行时提示与内置 Skill 文案中统一说明 `bash` 采用“直接传完整命令”契约。

#### Scenario: Prompt and skills align with new contract
- **WHEN** 系统注入工具运行时约束或读取相关 Skill 提示
- **THEN** 文案描述为“直接传完整命令”
- **AND** 不再出现“参数键必须是 command”约束
