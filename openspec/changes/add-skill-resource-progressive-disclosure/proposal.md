# Change: Add Skill Resource Progressive Disclosure

## Why
当前数字分身通过 `context__read(resource="skills", action="read", id="<skill_id>")` 只能读取 `SKILL.md`，无法继续按需读取同一 skill 下的 `references/*.md`。这使“标准 `SKILL.md` + references/scripts 渐进式披露”在本仓库内只完成了文件落地，未完成运行时可读协议。

同时，若直接把 skill 详情读取退回为 `bash` 自行 `cat/curl`，会破坏现有原生只读工具契约，降低路径稳定性，并重新引入 shell 转义、路径漂移与非结构化输出问题。

## What Changes
- 扩展 `context__read(resource="skills")` 的只读能力：
  - 保持 `skills.list` 与 `skills.read(id)` 现有语义不变；
  - 新增 `skills.index(id)`，返回指定 skill 的可读资源与可执行脚本索引；
  - 扩展 `skills.read(id, path)`，允许按白名单读取 `references/*.md` 等文本资源。
- 保持 skill 脚本不走 `context__read` 正文注入：
  - `scripts/*` 仅通过 `skills.index` 暴露发现信息；
  - 实际执行仍由 `bash` 完成，不新增第三个本地通用工具。
- 更新运行时提示与 skill 注入文案：
  - 明确“skill 文档读取优先走 `context__read`”；
  - 明确“skill 脚本执行走 `bash`，但仅在 skill 已声明且确有必要时执行”。
- 为 skill 资源读取增加显式边界：
  - 只允许读取 skill 根目录下的 `SKILL.md` 与 `references/*.md` 文本文件；
  - 禁止通过 skill 读接口任意路径穿越或直接读取脚本正文作为默认路径。

## Impact
- Affected specs:
  - `agent-native-api-tools`
  - `linux-bash-tool-contract`
- Affected code:
  - `internal/agent/context_read_tool.go`
  - `internal/agent/reply_generation.go`
  - `internal/agent/tool_runtime_prompt.go`
  - `internal/web/server_api_catalog.go`
  - `internal/skills/store_core.go`
  - `internal/skills/*test.go`
  - `internal/agent/*test.go`
