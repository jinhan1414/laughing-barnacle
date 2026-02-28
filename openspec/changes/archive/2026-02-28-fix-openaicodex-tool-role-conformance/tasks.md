## 1. Implementation
- [x] 1.1 在 `openai-codex` payload 归一化中实现 role 白名单映射，禁止输出 `role=tool`。
- [x] 1.2 保留工具结果消息内容，确保角色修正后工具上下文仍可继续参与后续推理。
- [x] 1.3 补充并更新 `openaicodex` adapter 单测，覆盖 `tool` 与未知 role 的归一化行为。

## 2. Validation
- [x] 2.1 运行 `go test ./internal/llmgateway/adapters/openaicodex -timeout 60s` 并通过。
- [x] 2.2 运行 `go test ./... -timeout 60s` 并通过。
- [x] 2.3 运行 `openspec validate fix-openaicodex-tool-role-conformance --strict` 并通过。
