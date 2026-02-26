## ADDED Requirements
### Requirement: JSON Maintenance APIs for MCP/Skills/Schedules
系统 MUST 为 MCP、Skills、Schedules 提供 JSON 维护写接口，并支持结构化参数校验。

#### Scenario: Save via JSON API
- **WHEN** 调用 `POST /api/mcp/services/save`、`POST /api/skills/save` 或 `POST /api/schedules/save` 且 JSON 参数合法
- **THEN** 系统完成写入并返回可解析的 JSON 成功响应
- **AND** 后续回读接口可见已生效配置

#### Scenario: Reject invalid JSON payload
- **WHEN** JSON body 缺失必填字段或格式错误
- **THEN** 系统返回明确错误状态与错误消息
- **AND** 不写入任何配置

### Requirement: Settings Compatibility over Shared Service
系统 MUST 保留现有 `/settings/*` 入口用于人工页面操作，同时与 JSON API 共享同一写入 service 逻辑。

#### Scenario: Same behavior for api and settings writes
- **WHEN** 用户通过 `/settings/*` 与模型通过 `/api/*` 写入同一配置对象
- **THEN** 两条路径产生一致的校验规则与持久化结果
- **AND** 不出现字段规则漂移

### Requirement: Maintenance Skills Use JSON Protocol
系统 MUST 将维护类 Skill 的写入示例统一为 JSON 协议，不再以表单 `--data-urlencode` 作为默认写入方式。

#### Scenario: Skill prompt uses JSON write examples
- **WHEN** 读取 `mcp-config-maintainer`、`skills-config-maintainer`、`schedule-config-maintainer`
- **THEN** 写操作示例使用 JSON body 与对应 `/api/*` 端点
- **AND** 保留写后回读校验步骤

### Requirement: Runtime Prompt Aligns with JSON Maintenance
系统 MUST 在工具运行时提示中明确维护接口默认 JSON 写入约束。

#### Scenario: Runtime prompt guides JSON maintenance
- **WHEN** 注入工具运行时约束到模型上下文
- **THEN** 维护接口规则包含 JSON 端点与 Content-Type 要求
- **AND** 不再将表单写入描述为默认路径
