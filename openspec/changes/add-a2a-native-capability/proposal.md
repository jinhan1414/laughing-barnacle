# Change: Add Native A2A Capability for Digital Twin

## Why
当前系统可稳定调用 `linux__bash` 与 MCP 工具，但缺少“数字分身调用其他 Agent”的原生执行通道。  
将 A2A 仅做成 Skill 会把协议执行退化为提示词+命令拼接，难以保证状态机一致性、认证安全与执行证据完整。

## What Changes
- 新增原生 `A2AProvider` 注入点，与 `MemoryProvider` 同级。
- 在内置工具层新增 `a2a__send`、`a2a__get`、`a2a__cancel`，由 `callBuiltinTool` 直接执行。
- 将 Skill 角色限定为“何时调用哪个 Agent”的策略提示，不承担 A2A 协议执行。
- 增加 A2A Agent 注册表（allowlist + 凭证配置），通过 `agent_id` 路由目标 Agent。
- 增加 A2A 执行证据字段，确保任务 ID、状态与结果可追踪。
- 提供本机 `codex` CLI 包装 Agent 的联调方案，作为 A2A POC。

## Impact
- Affected specs: `a2a-native-capability`
- Affected code:
  - `internal/agent/*`（Provider 注入、builtin tool 定义与分发）
  - `internal/a2a/*`（A2A client/provider/registry，新增模块）
  - `internal/mcp/store*.go` 或同等设置持久化层（A2A registry 配置落盘）
  - `internal/web/*`（A2A 设置与查询 API，若纳入设置页）
  - `README.md` / `docs/*`（运维与联调文档）
