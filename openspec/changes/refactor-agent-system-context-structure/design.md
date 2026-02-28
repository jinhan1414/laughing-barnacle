## Context
当前系统上下文由多个来源拼接：
- `DefaultSystemPrompt` 承载角色与回答策略；
- `responseStylePrompt` 在请求构建阶段再次注入；
- `buildToolRuntimePrompt` 承载工具环境、接口约束与执行硬规则；
- Skill/A2A/Memory/Async 索引采用渐进式披露按需读取详情。

该结构可用，但存在三类问题：
1. 规则分散且重复，模型注意力被冗余文案稀释；
2. `linux__bash` 的命名与 Windows PowerShell 执行现实存在语义冲突；
3. 维护写入在命令行层面需处理多层 JSON 转义，失败率高。

## Goals / Non-Goals
- Goals:
  - 让系统上下文结构模块化、可扫描、无重复。
  - 让模型明确 shell 真实执行环境，降低 Linux 语法误用。
  - 让维护写入与详情读取优先走结构化原生工具，避免命令行 JSON 转义。
  - 保持执行证据约束与渐进式披露原则，减少无效工具回合。
- Non-Goals:
  - 不将 `linux__bash` 改名为 `windows__powershell`（避免破坏现有契约与兼容链路）。
  - 不引入关键词/正则分流的决策逻辑。

## Decisions
- Decision: 上下文固定分块并保持顺序稳定
  - 统一块级结构，避免“同一策略在 system 多处重复”。
  - 将“回答策略”保留单一权威段落，移除同义重复注入。
  - 原因：提升可读性与 token 利用率，同时有利于请求前缀稳定。

- Decision: 保留 `linux__bash` 名称，但新增强制环境警戒
  - 在 Windows 场景显式声明“工具名为 `linux__bash`，但命令在 PowerShell 执行”。
  - 明确禁止输出 Bash 专属语法并固定 `curl.exe` 规则。
  - 原因：兼容现有工具契约，同时降低模型望文生义风险。

- Decision: 最小新增两个原生工具并按读写分离
  - 新增 `context__read`：封装只读查询能力，覆盖 Skill/A2A/Memory/Async 的索引与详情读取。
  - 新增 `maintenance__write`：封装维护写能力，覆盖 MCP/Skills/Schedules/A2A 的 save|toggle|delete|run|install。
  - 两个工具均采用结构化参数，由宿主完成 HTTP 组装与 JSON 序列化。
  - 原因：用最小工具数量消除命令行转义与 shell 语义漂移风险。

- Decision: 渐进式披露策略保持不变但迁移到原生读取工具
  - 详情读取默认走 `context__read`；
  - 单轮默认只拉取 1 个最相关详情，不足再补读。
  - 原因：保持低 token 开销，同时降低工具调用失败率。

## Risks / Trade-offs
- 风险：未改工具名，仍可能有少量语义误触发。
  - Mitigation：保留 `linux__bash` 仅用于本地 shell 任务，本地 API 交互默认切换到原生工具。

- 风险：上下文重排可能影响既有缓存命中。
  - Mitigation：固定段落顺序与标签，更新对应缓存稳定性测试。

- 风险：新增工具会带来接口映射维护成本。
  - Mitigation：工具内部只做轻量路由白名单，底层复用既有 `/api/*` service 与校验逻辑。

- 风险：约束过强可能压制模型自由表达。
  - Mitigation：只约束执行/接口/证据，不约束业务答案格式。

## Migration Plan
1. 先改 OpenSpec 规范，明确新上下文结构与“双原生工具”契约。
2. 新增 `context__read` 与 `maintenance__write`，并接入 Agent tool registry。
3. 重构 `DefaultSystemPrompt`、`responseStylePrompt`、`buildToolRuntimePrompt` 的分工。
4. 同步更新 Skill 文案与运行时约束，默认引导使用原生工具。
5. 通过单测/集成测试验证工具调用稳定性、前缀顺序与渐进式披露预算。

## Open Questions
- 无。
