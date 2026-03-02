## MODIFIED Requirements
### Requirement: Read-Only Contract for context__read
系统 MUST 将 `context__read` 限定为只读能力，并采用白名单资源与操作约束。

#### Scenario: Read skill detail by id
- **WHEN** 模型调用 `context__read` 且请求 `resource=skills`、`action=read`、`id=<skill_id>`
- **THEN** 系统返回该 skill 的 `SKILL.md` 详情数据
- **AND** 不执行任何写操作

#### Scenario: Read skill reference by id and path
- **WHEN** 模型调用 `context__read` 且请求 `resource=skills`、`action=read`、`id=<skill_id>`、`path=references/<file>.md`
- **THEN** 系统返回该白名单 reference 文件内容
- **AND** 不允许读取 skill 根目录外的文件

#### Scenario: Read skill resource index by id
- **WHEN** 模型调用 `context__read` 且请求 `resource=skills`、`action=index`、`id=<skill_id>`
- **THEN** 系统返回该 skill 的资源索引
- **AND** 索引至少包含 `SKILL.md`、可读 `references/*.md` 与已发现 `scripts/*`

#### Scenario: Reject invalid skill resource path
- **WHEN** 模型调用 `context__read` 请求 `resource=skills`、`action=read` 且 `path` 为绝对路径、包含 `..` 或不在白名单中
- **THEN** 系统返回显式参数错误
- **AND** 不读取任何文件

#### Scenario: Reject non-read action on context__read
- **WHEN** 模型调用 `context__read` 但传入写动作（如 save/delete/toggle）
- **THEN** 系统返回显式参数错误
- **AND** 不发起下游请求
