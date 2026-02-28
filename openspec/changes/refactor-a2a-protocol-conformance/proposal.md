# Change: 基于 A2A 官方 SDK 重构数字分身与 codex-a2a 参考服务

## Why
当前数字分身与 `integrations/codex-a2a` 的 A2A 能力主要依赖手写 JSON-RPC 封装，不符合“使用官方 SDK”的目标：
- 接入阶段没有使用官方 Agent Card 解析能力，字段回填与兼容策略不统一。
- Provider 执行链路未复用官方 SDK 的请求/响应模型，跨实现互操作风险高。
- `integrations/codex-a2a` 不是基于官方 SDK 的参考实现，难作为长期稳定联调基线。

这些问题会导致“能连通但不标准”，后续升级协议版本或接入第三方 Agent 时排障成本高。

## What Changes
- 对数字分身 A2A 执行链路做官方 SDK 化改造（不改变当前 async task 执行入口）：
  - 使用官方 `a2a-go` 作为 A2A 客户端与 Agent Card 解析主实现。
  - 在接入维护链路增加基于官方 SDK 的 Agent Card 发现与字段回填（显式报错、可回读）。
  - 按官方 Agent Card 契约校验必填字段，特别是 `skills`（对可执行 Agent 要求至少 1 个 skill）。
  - 在 provider 执行链路基于官方 SDK 返回结构做状态归一化，并保留原始状态证据。
- 强化执行证据字段，保证 A2A 问题可按“执行链路”回溯：
  - 至少包含 `agent_id/remote_task_id/status/raw_status/sdk_provider/sdk_method`。
- 升级 `integrations/codex-a2a` 为“官方 SDK 参考实现”：
  - 使用官方 `a2a-python` SDK 暴露 Agent Card 与任务生命周期接口。
  - 明确任务状态机与 `send/get/cancel` 契约。
  - 保持失败显式暴露，不引入 mock 成功路径或静默降级。
- 明确依赖约束：
  - 若引入 `a2a-go`，项目 Go 版本需满足该 SDK 最低要求（当前官方 README 标注为 Go 1.24.4+）。

## Impact
- Affected specs:
  - `a2a-native-capability`
  - `codex-a2a-reference-service`（新增）
- Affected code (expected):
  - `internal/mcp/store.go`
  - `internal/mcp/store_a2a_agents.go`
  - `internal/mcp/store_validation.go`
  - `internal/web/server_api_a2a_manage.go`
  - `internal/a2a/provider.go`
  - `internal/a2a/provider_rpc.go`
  - `internal/agent/async_task_manager_a2a.go`
  - `internal/agent/a2a_result_renderer.go`
  - `go.mod` / CI Go toolchain 配置（若升级 `a2a-go` 依赖）
  - `integrations/codex-a2a/codex_a2a_agent.py`
  - `integrations/codex-a2a/requirements*.txt`（新增 Python SDK 依赖时）
  - `integrations/codex-a2a/README.md`
  - A2A 相关测试文件
- Dependencies:
  - 与 `add-async-chat-task-orchestration` 兼容，执行入口继续保持 `async_task__submit(type=a2a)`。
