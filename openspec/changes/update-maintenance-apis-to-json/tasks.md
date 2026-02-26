## 1. Specification
- [ ] 1.1 明确 JSON 迁移范围：MCP、Skills、Schedules 三类维护写操作
- [ ] 1.2 明确兼容策略：保留 `/settings/*`，统一下沉 service 层
- [ ] 1.3 通过 `openspec validate update-maintenance-apis-to-json --strict`

## 2. Implementation
- [ ] 2.1 新增 MCP JSON 维护端点：`/api/mcp/services/save|toggle|delete`
- [ ] 2.2 新增 Skills JSON 维护端点：`/api/skills/save|toggle|delete|install`
- [ ] 2.3 新增 Schedules JSON 维护端点：`/api/schedules/save|toggle|delete|run`
- [ ] 2.4 抽取并复用 MCP/Skills/Schedules 共享写入 service（API/Settings 共用）
- [ ] 2.5 更新 `mcp-config-maintainer` 为 JSON 写入模板
- [ ] 2.6 更新 `skills-config-maintainer` 为 JSON 写入模板
- [ ] 2.7 更新 `schedule-config-maintainer` 为 JSON 写入模板
- [ ] 2.8 更新 runtime prompt 维护接口约束为 JSON 优先

## 3. Verification
- [ ] 3.1 单测：JSON body 参数校验、错误码与错误消息
- [ ] 3.2 单测：`/settings/*` 与 `/api/*` 调用共享逻辑且结果一致
- [ ] 3.3 单测：Skill 注入文本已切换 JSON 指令，不再包含 `--data-urlencode`
- [ ] 3.4 回归：`go test ./... -timeout 60s`
