## 1. Implementation
- [x] 1.1 在 `async_task__submit` -> A2A provider 发送链路中透传 `metadata`，并保留已有 `async_task_id` 注入。
- [x] 1.2 在 `codex-a2a` 执行器支持读取 `metadata.working_dir`，完成目录校验与回退逻辑。
- [x] 1.3 在 `codex-a2a` 中注入稳定且最小的默认执行提示词前缀，确保“持续执行到可交付结果”与“失败显式暴露”。
- [x] 1.4 调整 `codex exec` 调用参数为高权限 JSON 事件模式（`--dangerously-bypass-approvals-and-sandbox --json`），替换现有“`-o` 最后一条消息”路径。
- [x] 1.5 实现 JSONL 事件解析：提取 `turn.completed` 与最终 `agent_message`，据此决定 `completed` 或显式 `failed`。
- [x] 1.6 更新 `codex-a2a` 产物结构，保存最终答复与关键事件证据。
- [x] 1.7 更新 `integrations/codex-a2a/README.md`，补充权限模型、默认前缀、`working_dir` 用法、终态判定规则。

## 2. Validation
- [x] 2.1 新增/更新 Go 单测：验证 `metadata` 透传与 `async_task_id` 共存。
- [x] 2.2 新增/更新 `codex-a2a` 执行器测试：覆盖默认前缀注入、`turn.completed` 存在/缺失、最终消息缺失、CLI 非零退出等分支。
- [x] 2.3 回归 `go test ./...`（后端单测总超时 60s 约束内）。
- [x] 2.4 手工联调：提交含 `metadata.working_dir` 的 A2A 任务，验证终态与 artifact 证据一致。
