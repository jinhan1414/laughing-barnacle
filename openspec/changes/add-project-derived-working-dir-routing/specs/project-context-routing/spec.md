## ADDED Requirements
### Requirement: Single Root Derived Project Registry
系统 MUST 使用单一根目录配置与 `/projects` 相对路径模型派生所有项目工作目录，禁止在项目记忆中维护第二份绝对路径真相。

#### Scenario: Derive existing project workdir from root config
- **WHEN** `/projects` 中存在项目记录且配置文件中存在唯一 `root_dir`
- **THEN** 系统通过 `root_dir + relative_path` 派生项目 `working_dir`
- **AND** 不要求项目节点长期保存绝对路径字段

#### Scenario: Fail explicitly when root config is invalid
- **WHEN** 根目录配置缺失、为空或指向不可访问目录
- **THEN** 系统返回显式错误
- **AND** 禁止静默猜测或退回其他目录

### Requirement: Fixed Projects Index Injection
系统 MUST 在构建对话上下文时固定注入轻量的 `Projects Index`，作为本轮项目解析的主来源。

#### Scenario: Inject project index as fixed system context
- **WHEN** 本轮请求构建系统上下文且 `/projects` 下存在可用项目记录
- **THEN** 系统注入仅含 `project_id/working_dir/relative_path/aliases/summary` 等索引字段的 `Projects Index`
- **AND** 不将 `/projects` 全文或完整正文常驻注入

#### Scenario: Read project details on demand
- **WHEN** `Projects Index` 不足以确认目标项目
- **THEN** 模型按需通过 `context__read(resource="memory", action="read|section", path="...")` 读取单个项目详情
- **AND** 默认不把 `/projects` 全量读取作为固定首步

### Requirement: New Project Directory Derivation and Registration
系统 MUST 支持在用户发起新项目且未提供完整路径时，从唯一根目录派生新项目目录，并在创建成功后回写 `/projects`。

#### Scenario: Create new project from root-derived relative path
- **WHEN** 用户发起新项目任务且未提供完整工作目录
- **THEN** 系统基于项目名生成 `relative_path` 并派生 `working_dir`
- **AND** 在目录创建成功后将项目记录写回 `/projects`

#### Scenario: Expose directory creation conflict explicitly
- **WHEN** 目标 `relative_path` 已存在冲突且无法自动去重或创建
- **THEN** 系统返回显式错误
- **AND** 不伪造项目已创建成功
