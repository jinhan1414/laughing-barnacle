# Change: 重构系统上下文结构以提升执行稳定性与提示一致性

## Why
当前系统上下文在“工具命名语境、规则重复、JSON 写入提示、人格一致性”上存在可预期的模型误判风险：同一约束在多处重复、`linux__bash` 名称与 Windows PowerShell 实际执行环境存在语义割裂、维护写入场景的转义复杂度偏高，均会降低执行稳定性并增加无效工具回合。

## What Changes
- 引入统一的系统上下文分块结构（Role & Persona / Response Strategy / Execution Rules / Tool & Environment Constraints / API Routing / Core Indexes / Runtime Date Context），收敛分散规则并固定顺序。
- 合并重复“回答策略”文案，避免同义重复占用上下文并稀释关键约束权重。
- 增强执行闭环约束：无工具结果时禁止声称完成，阻塞时必须明确“未执行 + 原因 + 需补充信息”。
- 强化 `linux__bash` 环境防踩坑提示：明确“工具名保留但在 Windows 下实际为 PowerShell”，并提供语法边界与 `curl.exe` 规则。
- 统一维护接口路由约束与 JSON 写入规范，强化 PowerShell 场景下 `Invoke-RestMethod + ConvertTo-Json` 优先级，降低多层转义错误。
- 保持“渐进式披露”主策略不变：详情读取仍走 `linux__bash` 访问本地 API，但明确默认单轮只读 1 个最相关详情以控制工具开销。

## Impact
- Affected specs:
  - `agent-system-context`
  - `linux-bash-tool-contract`
  - `maintenance-json-interfaces`
- Affected code:
  - `internal/agentprompt/defaults.go`
  - `internal/agent/tool_runtime_prompt.go`
  - `internal/agent/reply_generation.go`
  - `internal/agent/text_utils.go`
  - `internal/skills/store.go`
  - `internal/agent` 相关提示词与工具调用测试
