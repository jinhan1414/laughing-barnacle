---
name: "定时任务配置维护"
description: "当用户要求查看/修改 Cron 定时任务时使用"
---

目标：以最少工具调用完成定时任务配置并可验证，默认预算 3-4 次调用。
写入字段硬约束：maintenance__write(resource="schedules", action="save") 的 payload 仅接受 id,name,description,action=skill:<skill_id>,cron_expr,enabled；禁止 cron/prompt/action=reminder。
硬性约束：本地 API 读写禁止走 bash；读取用 context__read，写入用 maintenance__write。
硬性约束：创建/更新 Skill 固定使用 maintenance__write(resource="skills", action="save")。
硬性约束：action 必须是 skill:<skill_id>；skill_id 仅允许 [a-zA-Z0-9_-]，必须使用普通连字符 '-'。
硬性约束：用户提醒类任务（如打卡/会议/出行提醒）禁止绑定流程性内置 skill（project-memory-maintainer、context-archive-recall、mcp-config-maintainer、skills-config-maintainer、schedule-config-maintainer）。
硬性约束：提醒类任务必须先创建或复用专用 reminder skill（例如 punch-card-reminder），再用 action=skill:<reminder_skill_id> 绑定。
硬性约束：定时任务列表固定使用 context__read(resource="schedules", action="list")。
步骤 1（必做）：先查技能：context__read(resource="skills", action="list")；仅在需要更新已有任务时再查一次 context__read(resource="schedules", action="list")。
步骤 2（按分支执行一次写入）：
  a) skill 不存在：先创建 skill：maintenance__write(resource="skills", action="save", payload={id,name,description,prompt,enabled})。
  b) skill 已存在但禁用：先启用：maintenance__write(resource="skills", action="toggle", payload={id,enabled:true})。
  c) 保存任务：maintenance__write(resource="schedules", action="save", payload={id,name,description,action,cron_expr,enabled})。
  d) 立即执行（仅用户明确要求时）：maintenance__write(resource="schedules", action="run", payload={id})。
步骤 3（必做）：回读一次：context__read(resource="schedules", action="list")；仅基于回读结果汇报是否生效。
失败处理约束：若接口调用失败，只允许重试同一 API 或先查 /healthz；禁止改为目录扫描、系统全盘搜索或 Linux 命令探测。
禁止写入引用不存在或未启用 skill 的 action。
Cron 规则使用 5 段：分 时 日 月 周（例如 30 8 * * *）。
默认自主闭环：时间与提醒目标明确时直接完成；仅在 cron、目标动作或提醒文案缺失时追问。
