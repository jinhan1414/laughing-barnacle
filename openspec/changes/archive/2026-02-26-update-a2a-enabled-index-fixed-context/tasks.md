## 1. Specification
- [x] 1.1 修改 `a2a-native-capability`：明确启用 A2A 索引为固定上下文主来源
- [x] 1.2 修改 `a2a-native-capability`：明确 `a2a-task-orchestrator` 默认不先读取 `/api/a2a/agents`
- [x] 1.3 通过 `openspec validate update-a2a-enabled-index-fixed-context --strict`

## 2. Implementation
- [x] 2.1 调整 A2A 索引系统提示文案，强调“先用索引，按需详情”
- [x] 2.2 更新 `data/skills/a2a-task-orchestrator/SKILL.md` 默认步骤为“索引优先”
- [x] 2.3 更新 `internal/skills/store.go` 的内置 A2A 编排 Skill 模板

## 3. Verification
- [x] 3.1 单测：A2A 索引注入文案符合“固定上下文主来源”约束
- [x] 3.2 单测：A2A 编排 Skill 文案不再要求首步读取 `/api/a2a/agents`
- [x] 3.3 回归：`go test ./... -timeout 60s`
