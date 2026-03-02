# Change: Add Native A2A Capability for Digital Twin

## Why
当前系统可稳定调用 `bash` 与 MCP 工具，但缺少“数字分身调用其他 Agent”的原生执行通道。  
将 A2A 仅做成 Skill 会把协议执行退化为提示词+命令拼接，难以保证状态机一致性、认证安全与执行证据完整。
此外，当用户提供新的 Agent 信息时，数字分身尚不能自主完成 A2A 接入登记，导致接入链路割裂。

## What Changes
- 新增原生 `A2AProvider` 注入点，与 `MemoryProvider` 同级。
- 在内置工具层新增 `a2a__send`、`a2a__get`、`a2a__cancel`，由 `callBuiltinTool` 直接执行。
- 新增两个 A2A 相关 Skill：
  - `a2a-config-maintainer`：维护接入（注册、更新、启停、删除、回读校验）
  - `a2a-task-orchestrator`：按用户目标选择并调用已接入 A2A Agent
- Skill 仅负责策略编排，不承担 A2A 协议执行。
- 增加 A2A Agent 注册表（allowlist + 凭证配置），通过 `agent_id` 路由目标 Agent。
- 维护链路统一走 JSON 受控接口（`/api/a2a/agents/save|toggle|delete`），支持“用户提供 agent 信息 -> 自动登记 -> 回读校验”。
- 在对话上下文新增 A2A 索引注入（渐进式披露）：首轮仅注入已接入 agent 索引与读取规则，详情按需读取。
- 增加 A2A 执行证据字段，确保任务 ID、状态与结果可追踪。
- 提供本机 `codex` CLI 包装 Agent 的联调方案，作为 A2A POC。

## Impact
- Affected specs: `a2a-native-capability`
- Affected code:
  - `internal/agent/*`（Provider 注入、builtin tool 定义与分发）
  - `internal/a2a/*`（A2A client/provider/registry，新增模块）
  - `internal/mcp/store*.go` 或同等设置持久化层（A2A registry 配置落盘）
  - `internal/skills/*`（新增 A2A 维护与使用 Skill）
  - `internal/web/*`（A2A 设置与查询 API，若纳入设置页）
  - `internal/agent/reply_generation.go`（A2A 索引渐进式披露注入）
  - `README.md` / `docs/*`（运维与联调文档）
