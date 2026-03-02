## 1. Spec
- [x] 1.1 新增 A2A 委托优先与工具预算临界点提示的 spec delta

## 2. Implementation
- [x] 2.1 强化运行时 prompt，明确大型开发/分析任务优先委托 A2A agent
- [x] 2.2 更新相关内置 Skill 文案，强调数字分身负责调度、A2A 负责执行
- [x] 2.3 在工具预算接近上限时注入“提前切 autonomous run”提示

## 3. Validation
- [x] 3.1 补充 prompt/skill 与工具预算提示测试
- [x] 3.2 运行 `go test ./internal/agent -timeout 60s`
- [x] 3.3 运行 `openspec validate update-a2a-delegation-and-early-run-handoff --strict`
