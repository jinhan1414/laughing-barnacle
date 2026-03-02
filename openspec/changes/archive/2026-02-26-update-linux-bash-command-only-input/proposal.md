# Change: 简化 bash 为命令直传

## Why
当前 `bash` 的调用参数是对象（`{"command":"...","timeout_sec":...}`），在真实对话中会产生多层转义与冗余键名，导致模型更容易生成不稳定参数。  
用户期望改为“只写完整命令”（如 `curl ...`），降低调用复杂度与 token 开销。

## What Changes
- 将 `bash` 的参数契约改为“单一命令字符串”，不再要求 `command` 键包装。
- `timeout_sec`、`working_dir` 不再暴露给模型调用面；运行时使用内置默认值（超时沿用当前默认）。
- 更新运行时提示与 Skill 文案，移除“参数键必须是 command”相关指令，改为“直接传完整命令”。
- 更新执行证据解析与相关测试，确保链路在新参数格式下仍可回读校验。
- **BREAKING**：旧对象参数格式将被显式拒绝，不做静默兼容。

## Impact
- Affected specs: `linux-bash-tool-contract`（新增）
- Affected code:
  - `internal/agent/linux_bash.go`
  - `internal/agent/tool_runtime_prompt.go`
  - `internal/agent/execution_evidence.go`
  - `internal/skills/store.go`
  - `data/skills/*.md`（仅涉及 bash 参数约束文案）
  - `internal/agent/*test.go`、`internal/skills/*test.go`
