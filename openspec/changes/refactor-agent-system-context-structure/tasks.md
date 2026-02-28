## 1. Spec & Prompt Contract
- [ ] 1.1 新增 `agent-system-context` 规范，定义分块结构、去重规则与执行证据约束。
- [ ] 1.2 新增 `agent-native-api-tools` 规范，定义 `context__read` 与 `maintenance__write` 的工具契约与白名单边界。
- [ ] 1.3 扩展 `linux-bash-tool-contract`：补充“工具名与真实 shell 环境对齐提示”以及“本地 API 交互默认走原生工具”要求。
- [ ] 1.4 扩展 `maintenance-json-interfaces`：补充维护写入优先走 `maintenance__write` 的要求。
- [ ] 1.5 运行 `openspec validate refactor-agent-system-context-structure --strict` 并修复所有校验问题。

## 2. Native Tools & Runtime Prompt Refactor
- [ ] 2.1 新增内置工具 `context__read`（只读）并接入 Agent 工具注册与参数校验。
- [ ] 2.2 新增内置工具 `maintenance__write`（只写）并复用现有 `/api/*` service 执行写入。
- [ ] 2.3 重构 `internal/agentprompt/defaults.go`，将系统提示改为固定 Heading 分块并移除重复回答策略。
- [ ] 2.4 调整 `internal/agent/reply_generation.go`，确保“回答策略”只保留单一注入来源且段落顺序稳定。
- [ ] 2.5 重构 `internal/agent/tool_runtime_prompt.go`，统一环境警戒、API 路由约束、执行证据硬规则与原生工具优先级。
- [ ] 2.6 同步 `internal/skills/store.go` 中维护类 Skill 文案，移除本地 API 读写的命令行说明，强制改为原生工具调用。

## 3. Validation
- [ ] 3.1 单测：验证请求 system 上下文中“回答策略”不重复注入。
- [ ] 3.2 单测：验证 Windows 场景下 runtime prompt 明确 `linux__bash` 与 PowerShell 的关系，且禁止其承接本地 API 读写。
- [ ] 3.3 单测：验证 `context__read` 仅允许白名单只读路由，非法路由显式报错。
- [ ] 3.4 单测：验证 `maintenance__write` 的路由、必填字段与 JSON body 校验行为。
- [ ] 3.5 单测：验证维护类提示词优先原生工具，并包含 JSON 协议约束。
- [ ] 3.6 单测：验证提示词中不存在“本地 API 可用 shell 命令写入”的文案。
- [ ] 3.7 回归 `go test ./...`（后端单测总超时 60 秒内）。
