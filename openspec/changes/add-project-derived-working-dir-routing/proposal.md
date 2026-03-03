# Change: add project-derived working directory routing

## Why
当前数字分身虽然可以通过 MemoryFS 读取 `/projects`，也可以向 `codex-local` 透传 `metadata.working_dir`，但两者没有形成稳定执行契约。结果是项目开发任务既无法稳定知道当前应在哪个目录执行，也无法保证 `codex-local` 对工作目录进行强校验。

## What Changes
- 新增 `project-context-routing` capability，定义单根目录派生、`/projects` 项目索引稳定注入、已有项目匹配与新项目目录派生规则。
- 修改 A2A 编排契约：涉及本地项目开发/分析时，数字分身必须先解析项目并派生 `working_dir`，再以 `metadata.working_dir` 委托 `codex-local`。
- 修改 `codex-a2a-reference-service`：`codex-local` 对执行型请求必须校验 `metadata.working_dir`，缺失或非法时显式失败，不再静默退回默认启动目录。

## Impact
- Affected specs: `project-context-routing`, `a2a-native-capability`, `codex-a2a-reference-service`
- Affected code:
  - `internal/agent/reply_generation.go`
  - `internal/agent/reply_resource_indexes.go`
  - `internal/agent/tool_runtime_prompt.go`
  - `internal/agent/async_task_tools.go`
  - `internal/agent/async_task_manager_a2a.go`
  - `internal/memory/*`
  - `integrations/codex-a2a/*`
