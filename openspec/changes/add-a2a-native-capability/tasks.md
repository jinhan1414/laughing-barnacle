## 1. Specification
- [ ] 1.1 确认 `a2a-native-capability` 的边界：仅原生执行通道，不替代 MCP
- [ ] 1.2 确认 A2A 双 Skill 分工：维护与使用分离，均不直接执行协议写入
- [ ] 1.3 确认 A2A 上下文注入遵循渐进式披露（仅索引，详情按需读取）
- [ ] 1.4 通过 `openspec validate add-a2a-native-capability --strict`

## 2. Implementation
- [ ] 2.1 在 `internal/agent` 增加 `A2AProvider` 接口、注入与 setter
- [ ] 2.2 增加 builtin tools：`a2a__register` / `a2a__send` / `a2a__get` / `a2a__cancel`
- [ ] 2.3 在 `callBuiltinTool` 中接入 A2A 调用与参数校验
- [ ] 2.4 新增 `internal/a2a` 模块（client/provider/registry）
- [ ] 2.5 增加 A2A registry 配置持久化与查询入口
- [ ] 2.6 保证 ToolCall 结果包含 `agent_id/task_id/status` 等执行证据
- [ ] 2.7 支持“用户提供 agent_card 信息后自动登记并返回 agent_id”
- [ ] 2.8 新增 Skill：`a2a-config-maintainer` 与 `a2a-task-orchestrator`
- [ ] 2.9 在上下文中注入 A2A 索引（与 Skill/Memory 同级渐进式披露）

## 3. Verification
- [ ] 3.1 单测：参数校验、provider 错误透传、状态映射
- [ ] 3.2 单测：无 provider / 未知 agent_id / 任务不存在等失败场景
- [ ] 3.3 单测：重复注册幂等、无效 AgentCard 拒绝写入
- [ ] 3.4 集成：用户提供 agent 信息后，完成 `register -> send -> get -> cancel` 联调
- [ ] 3.5 单测：A2A 索引注入仅包含摘要字段，不注入完整 AgentCard
- [ ] 3.6 回归：`go test ./...`（60 秒超时）
