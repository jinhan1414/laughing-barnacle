## Context
现状已经存在 A2A 索引注入（仅 enabled agent），但同时在系统提示与 `a2a-task-orchestrator` 中保留“先读 `/api/a2a/agents`”指导。  
这让模型即使已拿到索引仍倾向额外拉列表，无法稳定复用固定上下文。

## Goals / Non-Goals
- Goals:
  - 启用 A2A 索引成为任务编排默认输入，行为与 Skill 索引一致
  - 减少无必要列表查询，降低 token 与工具回合成本
  - 保留按需详情读取与显式刷新能力
- Non-Goals:
  - 不调整 A2A 写入维护接口（`save|toggle|delete`）语义
  - 不引入新的 fallback、mock 或静默降级
  - 不修改 A2A 内置工具集合（仍为 `a2a__send/get/cancel`）

## Decisions
### Decision 1: 固定注入索引作为默认事实来源
每轮请求继续注入已启用 A2A Agent 索引（`agent_id/name/description/status`），并将其定义为编排首选来源。  
模型默认直接从该索引选择 `agent_id`，而不是先调用 `/api/a2a/agents`。

### Decision 2: 按需读取详情，列表读取降级为例外
当索引信息不足以判定目标 Agent 时，才调用 `/api/a2a/agents/read?id=<agent_id>`。  
`/api/a2a/agents` 保留用于“用户明确要求刷新列表”或“执行前需做一致性校验”的场景，不作为固定第一步。

### Decision 3: Skill 文案与注入提示保持同构
`a2a-task-orchestrator` 的步骤描述改为“先用已注入索引，再按需读详情，再调用 `a2a__*`”。  
默认流程不再要求先 `curl /api/a2a/agents`，避免提示词和系统上下文相互冲突。

### Decision 4: 通过测试锁定行为
补充/调整测试，至少覆盖：
- A2A 索引系统提示包含“固定注入、优先使用索引”的行为约束
- A2A 编排 Skill 文案不再包含“步骤 1 必读列表”
- 仍保留按需详情读取提示与渐进式披露限制

## Risks / Trade-offs
- 风险：索引可能与最新 registry 之间存在瞬时滞后。  
  缓解：保留显式刷新列表入口，且执行前可按需做一致性校验。
- 风险：模型误把索引当作强一致实时状态。  
  缓解：在提示中明确“需要最新状态时再刷新列表”。

## Migration Plan
1. 更新注入提示与 A2A 编排 Skill 文案。
2. 调整对应单测断言。
3. 执行 `go test ./... -timeout 60s` 回归。
4. 通过 OpenSpec 严格校验后进入实现。
