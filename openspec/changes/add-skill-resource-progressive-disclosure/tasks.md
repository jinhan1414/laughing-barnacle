## 1. Spec
- [ ] 1.1 扩展 `agent-native-api-tools`，定义 `skills.index(id)` 与 `skills.read(id,path)` 的只读契约。
- [ ] 1.2 扩展 `linux-bash-tool-contract`，明确 skill 脚本执行仍走 `bash`，但 skill 文档读取不退回 shell。
- [ ] 1.3 运行 `openspec validate add-skill-resource-progressive-disclosure --strict`。

## 2. Implementation
- [ ] 2.1 扩展 skill store，支持生成单个 skill 的资源索引，并按白名单读取 `references/*.md`。
- [ ] 2.2 扩展 `/api/skills/read` 与新增 `/api/skills/index`，返回结构化 skill 资源信息。
- [ ] 2.3 扩展 `context__read(resource="skills")`，支持 `action=index` 及 `read + path`。
- [ ] 2.4 更新 runtime prompt 与 skill 索引提示，明确文档读取与脚本执行边界。

## 3. Validation
- [ ] 3.1 单测：`skills.index` 返回 `SKILL.md`、`references/*.md`、`scripts/*` 的结构化索引。
- [ ] 3.2 单测：`skills.read(id,path)` 仅允许读取白名单 references，非法路径显式拒绝。
- [ ] 3.3 单测：runtime prompt 明确“skill 文档用 `context__read`，脚本执行用 `bash`”。
- [ ] 3.4 回归 `go test ./internal/skills ./internal/agent ./internal/web`。
