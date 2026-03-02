---
name: "A2A 任务编排"
description: "当用户要求调用已接入外部 Agent 完成任务时使用"
---

目标：基于已接入 A2A Agent 完成任务编排与结果回读。
步骤 1：优先使用系统已注入的“已启用 A2A Agent 索引”选择最相关且 enabled=true 的 agent_id（默认不额外读取列表）。
步骤 2：如索引不足，再读单个详情：context__read(resource="a2a", action="read", id="<agent_id>")。
步骤 3：统一通过后台任务网关执行：async_task__submit(task_type=a2a, request, agent_id, agent_input)。
步骤 4：按需回读进度：async_task__get(task_id)；需要中断时使用 async_task__cancel(task_id)。
约束：async_task__submit.request 仅写稳定任务摘要，禁止“再次/重新/继续/再”等轮次词，以及“调用 codex-local”这类过程措辞。
约束：agent_input 直接描述目标任务与验收标准，禁止出现“调用 <agent_id>”“让 <agent_id> 执行”等调度语句。
约束：request 与 agent_input 语义一致但更短（建议 <= 60 字）；示例：request=“分析项目技术栈与风险”，agent_input=“分析 E:\\projects\\ai\\work-notiy，输出技术栈、启动构建、核心模块、风险与优化建议”。
约束：仅在用户明确要求刷新列表或执行前需要一致性校验时，才读取列表：context__read(resource="a2a", action="list")。
约束：禁止一次性读取全部 Agent 详情；单轮默认只读 1 个详情，不足再补读。
约束：禁止调用 a2a__send/a2a__get/a2a__cancel。
约束：只基于工具回读结果汇报，不得在无执行证据时声称“已完成”。
