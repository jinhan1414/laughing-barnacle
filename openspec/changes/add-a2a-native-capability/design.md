## Context
系统当前主链路是“用户输入 -> LLM 规划 -> 内置工具/MCP 执行 -> 工具结果回填 -> 最终回复”。  
若将 A2A 作为同级能力接入，需要满足：
- 不改变“失败显式暴露”的调试优先原则
- 不依赖关键词/正则分流
- 不让 Skill 承担协议执行

## Goals / Non-Goals
- Goals:
  - 提供可测试、可观测的 A2A 原生执行通道
  - 保持现有工具回合机制不破坏
  - 支持先用本机 `codex` CLI 包装 Agent 做联调验证
- Non-Goals:
  - 本变更不替换 MCP
  - 本变更不引入定时强制触发 A2A
  - 本变更不实现复杂多 Agent 编排 DSL

## Decisions
### Decision 1: 引入 A2AProvider 接口并同级注入 Agent
- 在 `internal/agent` 定义：
  - `type A2AProvider interface { Send(...); GetTask(...); CancelTask(...) }`
- 在 `Agent` 结构新增 `a2a A2AProvider` 与 `SetA2AProvider(...)`。
- 原因：把协议执行从提示词层抽离，保证可单测、可替换、可审计。

### Decision 2: A2A 作为内置工具，不走 MCP
- 新增 builtin tools：
  - `a2a__register(agent_card_url?, agent_card_json?, alias?, auth_token?)`
  - `a2a__send(agent_id, message, session_id?, metadata?)`
  - `a2a__get(agent_id, task_id)`
  - `a2a__cancel(agent_id, task_id)`
- 在 `callBuiltinTool` 优先分发，失败返回显式错误。
- 原因：与 `linux__bash` 同级，减少中间层依赖与映射损耗。

### Decision 3: 目标 Agent 仅允许 agent_id 路由
- 不允许模型直接传 URL；必须通过本地 registry 解析 `agent_id -> endpoint/auth`。
- registry 缺失或禁用时，返回明确错误。
- 原因：降低 SSRF 与错误路由风险，便于运维管理与审计。

### Decision 3.1: 用户提供 Agent 信息后支持自动登记
- 触发条件：用户明确表达“接入/添加该 Agent”并提供 `agent_card_url` 或 `agent_card_json`。
- 执行流程：
  1. 模型调用 `a2a__register`
  2. provider 拉取/解析 AgentCard 并校验必填字段
  3. 落盘到 registry，生成或返回稳定 `agent_id`
  4. 返回登记结果（`agent_id/name/base_url/enabled`）
- 幂等策略：
  - 同一 `agent_card_url` 或同一规范化 `base_url` 重复登记时返回既有 `agent_id`
  - 不生成重复记录
- 失败策略：校验失败、网络失败、持久化失败均显式报错，不做静默降级。

### Decision 4: Skill 仅提供策略，不执行协议
- Skill 可以提示“何时该调用哪个 `agent_id`”。
- Skill 不包含 A2A HTTP 调用脚本，不返回伪造执行结果。
- 原因：保持 Skill 渐进式披露定位，避免执行链路漂移到 prompt。

### Decision 5: 明确任务状态映射与证据回写
- `a2a__send/get/cancel` 统一返回结构化文本（含 `agent_id/task_id/status/artifacts/error`）。
- 将关键字段写入 `conversation.ToolCall.Result`，用于回放与追责。
- 原因：匹配现有“执行证据优先”策略。

### Decision 6: 注册校验与最小安全边界
- `a2a__register` 输入约束：
  - `agent_card_url` 与 `agent_card_json` 二选一，至少提供一个
  - 可选 `alias`；空值时回退 AgentCard `name`
- Provider 校验：
  - AgentCard 结构可解析
  - 必填能力字段完整（至少可发现通信端点）
  - URL 合法且协议受控（生产默认 `https`，本地开发允许 `http://127.0.0.1`/`http://localhost`）
- 不在 prompt 中保存明文密钥；若有 `auth_token`，写入配置存储并在展示层脱敏。

## Alternatives Considered
- 方案 A：通过 MCP bridge 接 A2A  
  - 优点：复用 MCP 管理面。  
  - 缺点：与本需求“非 MCP、同级能力”冲突。
- 方案 B：纯 Skill + linux__bash 调 A2A  
  - 优点：改动快。  
  - 缺点：稳定性差、难审计、易产生执行幻觉，拒绝采用。

## Risks / Trade-offs
- 风险：新增 builtin tool 后，模型可能过度调用 A2A。  
  - 缓解：沿用 `MaxToolCallRounds` 与失败显式反馈，不新增静默 fallback。
- 风险：长任务需要轮询，增加回合成本。  
  - 缓解：先提供 `send/get/cancel` 最小闭环，不在首版内引入复杂订阅。
- 风险：本机 `codex` CLI 包装 Agent 存在进程阻塞。  
  - 缓解：包装层必须支持超时、取消、非 0 退出码透传。

## Migration Plan
1. 先合入规格与接口骨架（不改变默认行为）。
2. 引入 A2A provider 与 builtin tools（含 `a2a__register`），默认可关闭。
3. 增加 registry 与设置入口（或配置文件入口）。
4. 增加“用户提供 Agent 信息 -> 自动登记”链路测试。
5. 增加 codex-local-a2a-agent 联调脚本与用例。
6. 回归测试通过后再开放给主流程使用。

## Open Questions
- A2A registry 首版放设置页还是仅环境变量/配置文件？
- `a2a__send` 首版是否支持 `await_terminal` 参数，还是坚持 `send + get` 分步？
- codex 包装 Agent 的输出协议是否需要强制 JSON schema？
