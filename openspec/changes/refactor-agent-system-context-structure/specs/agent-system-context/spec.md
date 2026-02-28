## ADDED Requirements
### Requirement: Structured System Context Blocks
系统 MUST 以固定 Heading 分块构建系统上下文，至少包含 `Role & Persona`、`Response Strategy`、`Execution Rules`、`Tool & Environment Constraints`、`API & Interface Routing`、`Core Indexes`、`[[RUNTIME_DATE_CONTEXT]]`。

#### Scenario: Build system context with canonical section order
- **WHEN** 服务在任意聊天轮次构建 system 上下文
- **THEN** 系统按固定顺序注入上述分块
- **AND** 同名分块在同一请求中只出现一次

### Requirement: Single Source of Response Strategy
系统 MUST 将“回答策略”维护为单一权威来源，禁止在多个 prompt 片段中重复注入同义策略文本。

#### Scenario: Response strategy is injected exactly once
- **WHEN** 构建发送给 LLM 的请求消息
- **THEN** “默认简洁直答、按需展开”的策略仅出现一次
- **AND** 不出现语义重复的并行版本

### Requirement: Persona Consistency with Strict Style
系统 MUST 在人设中明确“名字随性但执行风格严谨硬核”，并保持务实、直接、可靠的输出风格约束。

#### Scenario: Persona self-consistency line is present
- **WHEN** 生成默认 system 人设提示
- **THEN** 提示包含“名称与执行风格不冲突”的一致性描述
- **AND** 同时保留“禁止卖萌/表情符号/夸张语气”的硬约束

### Requirement: Execution Evidence Gating
系统 MUST 对执行型回复施加证据门控：无工具结果时不得宣称已执行或已完成。

#### Scenario: Block completion claims without tool output
- **WHEN** 本轮未产生 `function_call_output`
- **THEN** 系统约束模型不得输出“已执行/已完成”
- **AND** 若阻塞，必须明确“未执行”及阻塞原因

### Requirement: Progressive Disclosure Read Budget
系统 MUST 对 Skill/A2A/Memory 等详情读取采用渐进式披露预算，单轮默认只读取 1 个最相关详情，不足时再补读。

#### Scenario: Read one detail by default
- **WHEN** 用户请求命中某类索引且需要详情
- **THEN** 系统先引导模型只读取 1 个最相关详情
- **AND** 仅在信息不足时再继续读取更多详情

### Requirement: Native Tool Preference for Local API Interaction
系统 MUST 在本地 API 交互场景优先引导使用原生内置工具，而非让模型先拼接 shell 命令。

#### Scenario: Prefer native read/write tools in strategy prompt
- **WHEN** 系统注入执行策略与工具约束
- **THEN** 文案将本地 API 的读取与维护写入默认指向原生内置工具
- **AND** `bash` 仅作为 shell 任务或非 API 命令执行工具保留

### Requirement: Schema Responsibilities Stay in Tool Contracts
系统 MUST 将 API 参数结构、必填字段与格式校验职责下沉到工具 schema 与服务端校验，并让 system 提示词聚焦工具编排路径。

#### Scenario: Runtime prompt references routing, not full payload schema
- **WHEN** 系统注入 `Tool & Environment Constraints`
- **THEN** 文案优先描述“何时调用哪个工具”
- **AND** 不在提示词中展开高密度 payload 字段清单
