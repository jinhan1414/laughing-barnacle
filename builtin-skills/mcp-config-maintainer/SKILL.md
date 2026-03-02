---
name: "MCP 配置维护"
description: "当用户要求新增/修改/删除/启停 MCP 服务时使用"
---

目标：用最少工具调用维护 MCP 服务，默认预算 3-4 次调用。
硬性约束：本地 API 读写禁止走 bash；读取用 context__read，写入用 maintenance__write。
步骤 1（必做）：先查现状：context__read(resource="mcp", action="list")。
步骤 2（三选一，仅一次写入）：
  a) 新增/更新 HTTP 类 transport：maintenance__write(resource="mcp", action="save", payload={id,name,transport:"streamable_http|sse",endpoint,auth_token,headers,enabled})。
  b) 新增/更新 stdio：maintenance__write(resource="mcp", action="save", payload={id,name,transport:"stdio",command,args,env,enabled})。
  c) 启停/删除：maintenance__write(resource="mcp", action="toggle|delete", payload={id,enabled?})。
字段边界：stdio 仅使用 env；streamable_http / sse 仅使用 headers；headers 禁止写 Authorization。
更新语义：env / headers 留空表示不变，传 {} 表示清空。
步骤 3（必做）：回读校验：context__read(resource="mcp", action="list")，并仅基于回读结果汇报 diff。
默认自主闭环：目标明确时直接完成，不要求用户二次确认；仅在关键参数缺失、权限不足或删除对象歧义时追问。
