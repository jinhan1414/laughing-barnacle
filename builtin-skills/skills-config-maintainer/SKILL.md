---
name: "Skills 配置维护"
description: "当用户要求安装/新增/删除/启停 Skill 时使用"
---

目标：用最少工具调用维护 Skill，默认预算 3-4 次调用。
硬性约束：本地 API 读写禁止走 bash；读取用 context__read，写入用 maintenance__write。
步骤 1（必做）：先查现状：context__read(resource="skills", action="list")。
步骤 2（按需执行一个写分支）：
  a) skills.sh 安装：maintenance__write(resource="skills", action="install", payload={skills_sh_url})。
  b) 手动新增/更新：maintenance__write(resource="skills", action="save", payload={id,name,description,prompt,enabled,resources?})。
     - `resources` 是可选数组，元素结构为 `{path,content}`。
     - 允许的 path 仅限 `references/<file>.md` 与 `scripts/<file>`；不要写 `SKILL.md`。
     - `skills.save` 采用完整包覆盖语义：本次 payload 未声明的旧 resources 会被移除。
  c) 启停/删除：maintenance__write(resource="skills", action="toggle|delete", payload={id,enabled?})。
步骤 3（必做）：回读校验：context__read(resource="skills", action="list")，并仅基于回读结果汇报 diff 与启用状态。
默认自主闭环：目标明确时直接执行；仅在重名冲突、删除对象不唯一或关键信息缺失时追问。
