# Change: strengthen A2A delegation guidance and early autonomous-run handoff

## Why
当前系统虽然支持 A2A 与 autonomous run，但“大型开发/分析任务”仍可能由数字分身本体先自行执行，并在单轮工具预算耗尽后退化为无工具口头收尾。这与“数字分身负责调度，大任务优先交给匹配 A2A agent 执行”的产品预期不一致。

## What Changes
- 强化运行时 prompt 与内置 Skill 文案，让大型开发/分析类任务默认优先委托给已启用的 A2A agent。
- 在单轮工具预算接近上限时，向模型注入显式的“提前切 autonomous run”提示，优先提交后台 A2A 任务并写 checkpoint。
- 为上述行为补充自动化测试，覆盖 prompt 注入与预算临界点提示。

## Impact
- Affected specs: `a2a-native-capability`, `autonomous-run-orchestration`
- Affected code: `internal/agent/reply_generation.go`, `internal/agent/tool_runtime_prompt.go`, `builtin-skills/a2a-task-orchestrator/SKILL.md`, `builtin-skills/autonomous-run-orchestrator/SKILL.md`
