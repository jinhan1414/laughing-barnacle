## Context
`codex-a2a` 当前执行路径以“进程退出码 + `-o` 最后一条消息”为完成依据。实践中该依据不足以表达“任务已真正收敛”，会产生中间播报被误判为终态结果的问题。与此同时，固定工作目录会引入上下文串扰，不满足跨目录通用任务需求。

用户对该链路的明确要求是：
- Codex 执行应可访问用户完整磁盘；
- 不应引入特定任务结果契约；
- 完成判定应基于通用执行链路证据，而非业务内容模板。

## Goals / Non-Goals
- Goals:
  - 让 `codex-a2a` 默认具备全盘访问能力，支持通用本地任务。
  - 让任务完成判定依赖“回合终态证据”而非“最后一句文本”。
  - 让每次任务可指定工作目录，消除固定目录导致的上下文污染。
  - 维持错误显式暴露，不做静默降级或 mock 成功。
- Non-Goals:
  - 不引入按任务类型定制的输出 schema 契约。
  - 不改动主系统 A2A 状态映射基本集合（`working/completed/failed/canceled`）。

## Decisions
- Decision: 统一使用高权限执行模式运行 `codex exec`
  - 执行参数固定为 `--dangerously-bypass-approvals-and-sandbox`（语义等价 `approval=never + danger-full-access`），确保可访问完整磁盘。
  - 原因：满足“通用任务、完整磁盘访问”这一明确产品诉求。

- Decision: 追加最小默认执行提示词前缀
  - 在每次 `codex exec` 输入中拼接稳定前缀，前缀仅约束执行行为：
    - 禁止只输出计划后提前结束；
    - 必要时继续读取/执行直到形成可交付结果；
    - 出错需显式说明失败原因与关键证据。
  - 前缀不得强制业务字段模板，不得要求固定 JSON 结构。
  - 原因：提升任务收敛稳定性，同时保持通用任务能力。

- Decision: 以请求 metadata 指定本次工作目录
  - `async_task__submit.metadata.working_dir` 作为透传字段，经 A2A provider 传给 `codex-a2a`。
  - `codex-a2a` 仅做路径存在性与可访问性校验，不做关键词/正则抽取。
  - 原因：使用结构化状态而非文本启发式分流，符合项目长期规则。

- Decision: 以 `codex exec --json` 事件流定义“完成”
  - 解析 JSONL 事件，至少要求：
    - 出现 `turn.completed`；
    - 存在可作为最终输出的 `agent_message`。
  - 仅在上述证据齐全时将 A2A 任务标记 `completed`，否则显式 `failed`。
  - 原因：回合终态是协议级证据，适用于任意任务，不绑定业务模板。

- Decision: 保持输出通用，不引入业务结果契约
  - 不强制 `tech_stack` 等固定字段，不要求结果必须命中某些关键词。
  - 输出 artifact 以“最终答复 + 关键事件证据”为主。
  - 原因：避免约束通用 Agent 能力，保证后续任务可扩展。

## Risks / Trade-offs
- 风险：全盘访问提高误操作面。
  - Mitigation：保持显式日志与取消能力，必要时由部署环境提供外层隔离。

- 风险：JSONL 事件解析对 CLI 版本格式有依赖。
  - Mitigation：引入事件解析兼容测试；字段缺失时明确失败并暴露原始片段。

- 风险：默认提示词前缀过强可能影响部分任务自由度。
  - Mitigation：将前缀限制为最小行为约束，禁止引入业务输出格式要求；允许后续按配置微调文案。

- 风险：事件流内容较大，增加存储开销。
  - Mitigation：默认仅保存终态所需关键事件与摘要，完整流按配置开关保留。

## Migration Plan
1. 在 `internal/agent` 打通 `metadata.working_dir` 透传到 A2A provider 的链路。
2. 在 `codex-a2a` 中改造执行器：从 `-o` 终值判定切换为 `--json` 事件流判定。
3. 更新 `README` 与联调说明，明确权限模型与 metadata 字段。
4. 补齐单测/集成测试后灰度验证。

## Open Questions
- 无。
