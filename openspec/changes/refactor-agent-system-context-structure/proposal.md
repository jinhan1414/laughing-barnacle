# Change: 重构系统上下文结构以提升执行稳定性与提示一致性

## Why
当前系统上下文在“工具命名语境、规则重复、JSON 写入提示、人格一致性”上存在可预期的模型误判风险：同一约束在多处重复、工具命名与实际 shell 执行环境存在语义割裂、维护写入场景的转义复杂度偏高，均会降低执行稳定性并增加无效工具回合。

## What Changes
- 引入统一的系统上下文分块结构（Role & Persona / Response Strategy / Execution Rules / Tool & Environment Constraints / API Routing / Core Indexes / Runtime Date Context），收敛分散规则并固定顺序。
- 合并重复“回答策略”文案，避免同义重复占用上下文并稀释关键约束权重。
- 增强执行闭环约束：无工具结果时禁止声称完成，阻塞时必须明确“未执行 + 原因 + 需补充信息”。
- 强化 `bash` 环境防踩坑提示：明确其在 Windows 下由 PowerShell/cmd 承载执行，并限定其仅用于本地 shell 命令执行。
- 最小新增 2 个原生内置工具，替代本地 API 的命令行拼接交互：
  - `context__read`：仅负责 Skills/A2A/Memory/Async 索引与详情读取（只读白名单）。
  - `maintenance__write`：仅负责 MCP/Skills/Schedules/A2A 的 save|toggle|delete|run|install 写操作（JSON body 结构化入参）。
- 移除本地 API 读写的旧兼容路径：不再将 `bash + curl/Invoke-RestMethod` 作为可选执行链路。
- 保持“渐进式披露”主策略不变：详情读取改为优先走 `context__read`，默认单轮只读 1 个最相关详情，不足再补读。

## Impact
- Affected specs:
  - `agent-system-context`
  - `agent-native-api-tools`
  - `bash-tool-contract`
  - `maintenance-json-interfaces`
- Affected code:
  - `internal/agent/tools/context_read_tool*.go`
  - `internal/agent/tools/maintenance_write_tool*.go`
  - `internal/agentprompt/defaults.go`
  - `internal/agent/tool_runtime_prompt.go`
  - `internal/agent/reply_generation.go`
  - `internal/agent/call_tool*.go`
  - `internal/web/*`（复用现有 `/api/*` 服务层）
  - `internal/skills/store.go`（文案迁移到原生工具调用）
  - `internal/agent` 相关提示词与工具调用测试
