---
name: "A2A 接入维护"
description: "当用户要求新增/修改/删除/启停 A2A Agent 接入时使用"
---

目标：用最少工具调用维护 A2A 接入，默认预算 3-4 次调用。
硬性约束：本地 API 读写禁止走 bash；读取用 context__read，写入用 maintenance__write。
步骤 1（必做）：先查现状：context__read(resource="a2a", action="list")。
步骤 2（按需执行一个写分支）：
  a) 新增/更新：maintenance__write(resource="a2a", action="save", payload={id,name,description,endpoint,agent_card_url,auth_token,enabled})。
  b) 启停：maintenance__write(resource="a2a", action="toggle", payload={id,enabled})。
  c) 删除：maintenance__write(resource="a2a", action="delete", payload={id})。
步骤 3（必做）：回读校验：context__read(resource="a2a", action="list")；必要时再读详情：context__read(resource="a2a", action="read", id="<agent_id>")。
默认自主闭环：目标明确时直接执行；仅在 endpoint/card_url 缺失、删除对象歧义或鉴权信息缺失时追问。
