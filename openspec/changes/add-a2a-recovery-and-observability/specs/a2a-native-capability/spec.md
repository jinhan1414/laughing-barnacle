## MODIFIED Requirements
### Requirement: Official SDK Based A2A Invocation
系统 MUST 使用 A2A 官方 SDK 实现数字分身到远端 Agent 的调用链路，禁止以手写 JSON-RPC/HTTP 作为主执行实现。

#### Scenario: Use official SDK as primary invocation path
- **WHEN** 数字分身发起 A2A 调用
- **THEN** provider 通过官方 SDK 完成请求构造、发送与响应解析
- **AND** 非调试场景不走手写 JSON-RPC 直连路径

#### Scenario: Return explicit error when SDK invocation is unavailable
- **WHEN** 官方 SDK 初始化失败、版本不兼容或调用异常
- **THEN** 系统返回显式错误
- **AND** 不伪造成功结果

#### Scenario: Query task without mandatory agent card rediscovery
- **WHEN** 系统对已登记的 A2A Agent 执行 `GetTask` 或 `CancelTask`
- **THEN** 查询链路优先使用已保存 endpoint 建立 SDK client
- **AND** 不要求每次状态查询都重新解析 Agent Card

## ADDED Requirements
### Requirement: Recoverable A2A Tracking After Polling Errors
系统 SHALL 在 A2A tracker 因临时轮询错误暂停后，按策略自动尝试恢复跟踪，而不是永久停在 `paused`。

#### Scenario: Resume paused task automatically after temporary outage
- **WHEN** A2A 任务因连续轮询错误进入 `paused`
- **THEN** 系统记录下一次恢复时间并异步调度恢复 worker
- **AND** 远端恢复可达后，任务状态继续推进到最新状态

#### Scenario: Keep explicit evidence for repeated recovery failures
- **WHEN** 自动恢复后远端仍不可达
- **THEN** 系统保留 `paused/recovering` 相关日志与计数证据
- **AND** 不伪造任务成功或静默吞掉错误
