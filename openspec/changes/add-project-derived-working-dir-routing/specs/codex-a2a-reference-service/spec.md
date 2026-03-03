## ADDED Requirements
### Requirement: Mandatory Working Directory Validation for codex-a2a
`integrations/codex-a2a` MUST 在执行型请求开始前校验 `metadata.working_dir` 必填且合法，禁止在缺失该字段时退回默认启动目录执行。

#### Scenario: Accept task with valid working_dir
- **WHEN** A2A 请求携带可访问且为目录的 `metadata.working_dir`
- **THEN** `codex-a2a` 使用该目录执行 `codex exec`
- **AND** 产物证据中记录实际 `working_dir`

#### Scenario: Reject task without working_dir
- **WHEN** A2A 执行型请求缺失 `metadata.working_dir`
- **THEN** `codex-a2a` 返回显式错误
- **AND** 不启动 `codex exec`

#### Scenario: Reject task with invalid working_dir
- **WHEN** `metadata.working_dir` 为空、路径不存在、不是目录或不可访问
- **THEN** `codex-a2a` 返回显式错误
- **AND** 不退回默认工作目录继续执行
