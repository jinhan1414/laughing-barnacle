---
name: "Autonomous Run Orchestrator"
description: "当用户要求数字分身在一个多步骤目标上无人值守自动推进，且需要在 async_task/A2A 终态后自动继续下一步时使用"
---

目标：把“多步自动任务”编排成结构化的 autonomous run，而不是只提交一次后台任务后停住。

规则：
- 当任务需要跨多个步骤持续推进，且某一步可能转后台时，使用本技能。
- 当任务主体应交给 A2A Agent 执行时，数字分身负责调度和 checkpoint，不负责在当前单轮内继续展开主体开发。
- 若当前步骤会等待后台结果，先调用 `async_task__submit`，再调用 `autonomous_run__checkpoint`。
- 若当前工具预算已接近上限，优先在本轮提交后台任务并写 checkpoint，禁止继续扩张本地执行步骤。
- `autonomous_run__checkpoint` 必须写清：
  - `goal`
  - `status`
  - `current_step`
  - 等待异步时的 `waiting_ref`
- 恢复执行时，必须继续复用同一个 `run_id`，禁止重新创建新的 run。
- 若需要用户补充输入，写 `waiting_human`；若任务完成，写 `completed`；若无法继续，写 `failed` 并附错误。
- 不要只说“我会继续处理”；必须通过 checkpoint 工具把状态写入系统。
