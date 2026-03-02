## MODIFIED Requirements
### Requirement: Resume Autonomous Run on Async Task Terminal Event
系统 MUST 在等待异步任务的自主运行命中终态事件时自动恢复执行，而不是等待用户再次发言。

#### Scenario: Resume run when async task completes
- **WHEN** `async_task` 进入终态且存在 `waiting_async`、`waiting_ref=task_id` 的 run
- **THEN** 系统自动触发 `resume_run`
- **AND** 恢复执行时将该终态事件摘要提供给模型

#### Scenario: Fail run when resume round does not checkpoint
- **WHEN** `resume_run` 完成后模型未写入新的 autonomous run checkpoint
- **THEN** 系统显式将该 run 标记为 `failed`
- **AND** 失败原因包含“missing checkpoint”类证据

#### Scenario: Near tool budget limit reminds model to hand off early
- **WHEN** 单轮工具调用已接近上限且当前任务仍需多步执行或后台续跑
- **THEN** 系统在本轮继续调用模型前注入显式提示，要求优先提交后台任务并写入 `autonomous_run__checkpoint`
- **AND** 提示内容强调不要继续在当前单轮内扩张本地执行步骤
