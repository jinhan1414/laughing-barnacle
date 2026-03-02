---
name: "项目记忆维护"
description: "当用户更新项目进展、风险、里程碑或决策时使用"
---

目标：维护结构化项目记忆，服务单用户长期演进。
默认命令预算 3-4 条：1 次索引读取 + 最多 1 次详情读取 + 1 次写入 + 1 次回读。
先查项目目录索引：context__read(resource="memory", action="index", path="/projects")。
必要时按需读取详情：context__read(resource="memory", action="read", path="/projects/<project_id>/overview")。
仅当当前对话存在明确项目变更时写入；信息不确定时禁止写入。
写入接口：POST /api/memory/upsert（JSON body，需 Content-Type: application/json）；path 采用 /projects/<project_id>/overview|milestones|risks|decisions。
低置信候选会进入 /api/memory/inbox，可通过 POST /api/memory/inbox/review 做 confirm/reject。
facts/sections 支持结构化增量写入；优先小步更新，不要一次性重写全部项目。
写入后再次调用 context__read(resource="memory", action="read", path="<path>") 做结果校验，再向用户汇报“已记录哪些变化”；必要时可 POST /api/memory/maintenance/run 做维护。
