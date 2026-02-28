## ADDED Requirements
### Requirement: Two Minimal Native API Tools
系统 MUST 仅新增两个原生内置 API 工具：`context__read` 与 `maintenance__write`，并保持读写职责分离。

#### Scenario: Expose exactly two dedicated API tools
- **WHEN** Agent 构建可调用工具列表
- **THEN** 工具集中包含 `context__read` 与 `maintenance__write`
- **AND** 不为相同用途再新增第三个本地 API 通用工具

### Requirement: Read-Only Contract for context__read
系统 MUST 将 `context__read` 限定为只读能力，并采用白名单资源与操作约束。

#### Scenario: Read skill detail by id
- **WHEN** 模型调用 `context__read` 且请求 `resource=skills`、`action=read`、`id=<skill_id>`
- **THEN** 系统返回对应详情数据
- **AND** 不执行任何写操作

#### Scenario: Reject non-read action on context__read
- **WHEN** 模型调用 `context__read` 但传入写动作（如 save/delete/toggle）
- **THEN** 系统返回显式参数错误
- **AND** 不发起下游请求

### Requirement: Write Contract for maintenance__write
系统 MUST 将 `maintenance__write` 限定为维护写入能力，并通过结构化参数映射到既有 JSON 维护接口。

#### Scenario: Save schedule through maintenance__write
- **WHEN** 模型调用 `maintenance__write` 且 `resource=schedules`、`operation=save`、`payload` 合法
- **THEN** 系统执行对应维护写入并返回结构化结果
- **AND** 写入请求采用 `Content-Type: application/json`

#### Scenario: Enforce required fields by resource and operation
- **WHEN** `maintenance__write` 收到缺失必填字段的 payload
- **THEN** 系统返回显式校验错误
- **AND** 不执行写入
