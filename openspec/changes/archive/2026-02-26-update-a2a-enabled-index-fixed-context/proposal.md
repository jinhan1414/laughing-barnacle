# Change: 将启用 A2A Agent 索引固定暴露为 LLM 上下文主来源

## Why
当前系统已经会注入 A2A 索引，但 `a2a-task-orchestrator` 仍把“先 `curl /api/a2a/agents`”作为默认步骤。  
这会导致每轮额外工具调用、增加 token 与时延，也与“像 Skill 索引那样先用固定上下文”的目标不一致。

## What Changes
- 明确“已启用 A2A Agent 索引”是回复阶段固定注入的 LLM 上下文主来源（与 Skill 索引一致）。
- 约束任务编排默认流程：
  - 先基于已注入索引选择 `agent_id`
  - 索引不足时再按需读取单个详情（`/api/a2a/agents/read?id=<agent_id>`）
  - 不再将 `/api/a2a/agents` 作为每轮必做步骤
- 保留列表读取接口用于显式刷新或一致性校验场景，不作为默认首步。
- 同步更新 A2A 任务编排 Skill 文案与对应测试，确保执行链路与规格一致。

## Impact
- Affected specs: `a2a-native-capability`
- Affected code:
  - `internal/agent/reply_generation.go`
  - `internal/skills/store.go`
  - `data/skills/a2a-task-orchestrator/SKILL.md`
  - `internal/agent/agent_test_a2a_test.go`
  - `internal/skills/*test.go`
