## ADDED Requirements
### Requirement: Project-Derived Working Directory for Async A2A Dispatch
系统 MUST 在本地项目开发或分析任务委托给 `async_task__submit(task_type=a2a)` 时，先解析目标项目并派生 `working_dir`，再通过 `metadata.working_dir` 透传给远端 Agent。

#### Scenario: Delegate existing project task with derived working_dir
- **WHEN** 用户要求对已登记项目执行开发或分析，且模型选择 `codex-local` 承接主体执行
- **THEN** 系统在提交 A2A 任务前从 `Projects Index` 或对应项目详情派生 `working_dir`
- **AND** 提交给 provider 的 `metadata` 中包含 `working_dir`

#### Scenario: Reject project delegation without resolved working_dir
- **WHEN** 项目型 A2A 委托无法解析唯一 `working_dir`
- **THEN** 系统返回显式错误或向用户补充询问
- **AND** 不向 `codex-local` 发起缺失 `working_dir` 的执行请求
