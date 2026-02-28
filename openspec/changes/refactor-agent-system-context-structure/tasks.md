## 1. Spec & Prompt Contract
- [ ] 1.1 新增 `agent-system-context` 规范，定义分块结构、去重规则与执行证据约束。
- [ ] 1.2 扩展 `linux-bash-tool-contract`：补充“工具名与真实 shell 环境对齐提示”要求。
- [ ] 1.3 扩展 `maintenance-json-interfaces`：补充 PowerShell JSON 写入优先策略与禁用手写复杂转义默认路径。
- [ ] 1.4 运行 `openspec validate refactor-agent-system-context-structure --strict` 并修复所有校验问题。

## 2. Runtime Prompt Refactor
- [ ] 2.1 重构 `internal/agentprompt/defaults.go`，将系统提示改为固定 Heading 分块并移除重复回答策略。
- [ ] 2.2 调整 `internal/agent/reply_generation.go`，确保“回答策略”只保留单一注入来源且段落顺序稳定。
- [ ] 2.3 重构 `internal/agent/tool_runtime_prompt.go`，统一环境警戒、API 路由约束、执行证据硬规则。
- [ ] 2.4 同步 `internal/skills/store.go` 中维护类 Skill 文案，保持 JSON 写入与 PowerShell 约束一致。

## 3. Validation
- [ ] 3.1 单测：验证请求 system 上下文中“回答策略”不重复注入。
- [ ] 3.2 单测：验证 Windows 场景下 runtime prompt 明确 `linux__bash` 与 PowerShell 的关系及 `curl.exe` 约束。
- [ ] 3.3 单测：验证维护类提示词包含 JSON 写入路由与 `Content-Type: application/json` 约束。
- [ ] 3.4 回归 `go test ./...`（后端单测总超时 60 秒内）。
