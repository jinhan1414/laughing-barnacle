# Change: 重构 codex-a2a 运行语义以支持全盘任务与可靠完成判定

## Why
当前 `integrations/codex-a2a` 以 `codex exec -o <file>` 的最后一条消息作为产物，并在进程返回码为 0 时直接标记 `completed`。该行为会把“过程播报”误判为终态结果，造成“任务状态成功但业务结果未完成”。

同时，服务固定使用单一工作目录，远端任务容易被当前仓库上下文（如本地规则文件）干扰；且默认执行权限不足以覆盖“可访问用户完整磁盘”的任务诉求。

## What Changes
- 将 `codex-a2a` 的执行权限调整为全盘可访问模式（语义等价于 `approval=never` + `sandbox=danger-full-access`，在 `codex exec` 中采用 `--dangerously-bypass-approvals-and-sandbox` 实现）。
- 引入请求级工作目录机制：通过 A2A 请求 `metadata` 传递 `working_dir`，由 `codex-a2a` 解析并作为本次 `codex exec` 的 `-C`。
- 引入最小默认执行提示词前缀：约束“不要只停在计划、需要继续执行到可交付结果、失败显式返回”，但不约束业务输出格式。
- 将完成判定从“进程成功退出”改为“存在完整回合终态证据”：基于 `codex exec --json` 事件流识别 `turn.completed` 与最终 `agent_message`。
- 保持通用任务能力：不引入固定业务结果契约（不强制特定 schema/关键词）。
- 补充执行证据落盘：终态结果同时记录最终答复与必要的事件流证据，便于链路排障。

## Impact
- Affected specs:
  - `codex-a2a-reference-service`
  - `a2a-native-capability`
- Affected code:
  - `integrations/codex-a2a/codex_a2a_agent.py`
  - `integrations/codex-a2a/README.md`
  - `internal/agent/async_task_manager_a2a.go`
  - `internal/agent/async_task_tools.go`
  - `internal/agent` 中 A2A 相关测试文件
