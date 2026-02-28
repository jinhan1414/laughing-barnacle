## Context
代码现状已经将 A2A 执行入口收敛到后台任务网关（`async_task__submit(type=a2a)`），并由后台运行时轮询远端任务。现有问题不是“有没有能力”，而是“没有使用官方 SDK 作为协议实现基础”：
- 接入维护未复用官方 Agent Card 解析能力。
- provider 为手写 JSON-RPC 调用栈，缺少官方 SDK 的模型与兼容保障。
- `integrations/codex-a2a` 不是官方 SDK 实现，参考价值有限。

## Goals / Non-Goals
- Goals:
  - 在不改变执行入口的前提下，将 A2A 协议实现替换为官方 SDK 路线。
  - 提高跨实现 A2A Agent 互操作性与排障证据完整性。
  - 将 `codex-a2a` 改造成官方 SDK 的本地参考服务。
- Non-Goals:
  - 不引入新的执行入口（不恢复 `a2a__send/get/cancel` 直连入口）。
  - 不在本提案中引入 SSE/Webhook 的全新传输链路。
  - 不改动系统提示词大框架，仅做与新契约一致的必要更新。

## Decisions
### Decision 1: 执行入口保持 async task 网关，协议改造聚焦 SDK 替换
A2A 调用仍由 `async_task__submit(type=a2a)` 发起，避免与当前主干行为冲突；本次重构限定在接入维护、协议发现、provider 执行与证据结构。

### Decision 2: 数字分身 A2A 客户端固定使用官方 `a2a-go`
Go 侧 provider 不再直接手写 JSON-RPC 请求，改为官方 `a2a-go` 的客户端调用与类型模型。对 SDK 不支持或协议不兼容的目标，返回显式错误。

### Decision 3: 接入阶段使用官方 SDK 做 Agent Card 发现并强校验 `skills`
当用户通过 `/api/a2a/agents/save` 提供 `agent_card_url` 时，服务端通过官方 SDK resolver 读取 Agent Card，并将标准字段回填到 registry。发现结果必须包含 `skills` 字段；对可执行 Agent，`skills` 不能为空。发现失败或校验失败时显式返回错误。

### Decision 4: 状态流转以官方 SDK 结果为主，保留本地归一化映射
Provider 先消费官方 SDK 的任务状态，再映射到本地异步任务状态；同时保留 `raw_status` 作为执行证据。未知状态显式暴露，禁止静默成功。

### Decision 5: codex-a2a 参考服务固定使用官方 `a2a-python` SDK
`integrations/codex-a2a` 不再作为手写 HTTP/JSON-RPC 服务，而改为官方 Python SDK 服务实现。任务执行器仍可调用本地 `codex` CLI，但 A2A 协议层由 SDK 承担。

### Decision 6: 接受 Go 版本基线变更
官方 `a2a-go` 当前要求 Go 1.24.4+，与项目现有 Go 1.22+ 基线存在差异。方案中将该升级作为显式前置条件，并同步调整 CI/构建环境。

## Risks / Trade-offs
- 风险：升级 Go 版本可能影响既有构建链路。
  - 缓解：先做 toolchain 升级与回归，再切换 provider 实现。
- 风险：Agent Card 发现引入网络依赖，可能增加接入失败率。
  - 缓解：失败显式返回，并允许用户走“手动字段直填”模式。
- 风险：状态归一化规则过严可能暴露更多错误。
  - 缓解：这是调试优先策略，保持错误可见，不做静默吞错。

## Migration Plan
1. 升级 Go toolchain 与 CI 基线，满足 `a2a-go` 依赖要求。
2. 接入 `a2a-go` 客户端与 Agent Card resolver，替换手写 provider 调用层。
3. 增加 SDK 驱动的状态归一化与执行证据字段，补齐单测。
4. 将 `integrations/codex-a2a` 重写为 `a2a-python` SDK 服务。
5. 全量回归 `go test ./... -timeout 60s` 与 `codex-a2a` 联调用例。

## Open Questions
- 是否确认项目 Go 版本基线升级到官方 `a2a-go` 所需版本（当前文档标注 1.24.4+）。
- `agent_card_url` 发现失败时，默认策略是“强失败拒绝保存”还是“允许显式 `skip_discovery=true` 继续保存”。
