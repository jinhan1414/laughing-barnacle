## 1. Implementation
- [x] 1.1 升级项目 Go toolchain/CI 到 `a2a-go` 依赖要求版本，并完成构建验证。
- [x] 1.2 引入官方 `a2a-go` 依赖，替换当前手写 A2A provider 请求实现。
- [x] 1.3 在 `/api/a2a/agents/save` 接入官方 SDK Agent Card resolver，实现发现与字段回填（含显式失败路径）。
- [x] 1.8 在接入校验中增加 Agent Card `skills` 字段约束（缺失或空 skills 显式拒绝）。
- [x] 1.4 增加 SDK 结果到本地异步任务状态的归一化映射，并保留 `raw_status`。
- [x] 1.5 升级 A2A 执行证据落盘字段，至少包含 `sdk_provider/sdk_method`。
- [x] 1.6 将 `integrations/codex-a2a` 改造为官方 `a2a-python` SDK 服务实现。
- [x] 1.7 更新 `integrations/codex-a2a` 启动脚本与 README，明确官方 SDK 依赖与运行方式。

## 2. Validation
- [x] 2.1 单测：A2A 保存接口通过官方 resolver 在 `agent_card_url` 可达/不可达/字段缺失时行为符合契约。
- [x] 2.7 单测：Agent Card 缺失 `skills` 或 `skills` 为空时，接入接口返回显式错误且不落库。
- [x] 2.2 单测：Provider 走官方 SDK 调用链，SDK 初始化/调用失败返回显式错误。
- [x] 2.3 单测：远端状态映射覆盖 `submitted/working/input-required/auth-required/completed/failed/canceled/rejected`。
- [x] 2.4 单测：执行证据包含 `agent_id/remote_task_id/status/raw_status/sdk_provider/sdk_method`。
- [x] 2.5 集成验证：数字分身对接改造后的 `a2a-python` 版 `codex-a2a` 可完成提交、查询、取消闭环。
- [x] 2.6 回归：`go test ./... -timeout 60s`。

## 3. Spec
- [x] 3.1 完成 `a2a-native-capability` 的增量规范并通过严格校验。
- [x] 3.2 新增 `codex-a2a-reference-service` 规范并通过严格校验。
- [x] 3.3 执行 `openspec validate refactor-a2a-protocol-conformance --strict` 并修复全部问题。
