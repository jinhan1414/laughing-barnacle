---
name: "后台任务编排"
description: "当任务耗时较长或需要回合外持续跟踪时使用"
---

目标：由模型自主决定是否将任务转后台，并通过内置 async_task 工具闭环执行。
执行入口固定：async_task__submit；查询入口：async_task__get；取消入口：async_task__cancel。
submit 参数硬约束：task_type 与 request 必填；当 task_type=a2a 时，agent_id 与 agent_input 必填。
submit 文案约束：request 仅写稳定任务摘要，禁止“再次/重新/继续/马上”等轮次词；详细要求写入 agent_input 或后续工具参数。
submit 文案约束：task_type=a2a 时，agent_input 禁止“调用 <agent_id>”类调度语句，必须直接描述要执行的业务任务。
默认 notify_on_finish=true，任务终态后系统会主动通知用户。
仅在需要排障时才读取日志窗口：async_task__get(include_logs=true, log_cursor, log_limit<=200)。
禁止通过 Skill 直接发 HTTP 执行后台任务。
禁止在无执行证据时声称“已完成”。
