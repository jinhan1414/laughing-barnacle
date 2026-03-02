---
name: "上下文归档召回"
description: "当历史摘要信息不足，需要回看被压缩原文片段时使用"
---

仅在当前摘要无法支撑回答时触发，且必须按需最小化读取（单轮最多 3 条命令：1 次索引 + 最多 2 次分节）。
步骤 1：先调用 context__read(resource="memory", action="read", path="/conversation/archive/<segment_id>/index") 读取归档索引（标题与分节）。
步骤 2：根据问题只选择必要 section_id，再调用 context__read(resource="memory", action="section", path="/conversation/archive/<segment_id>/index", section_id="<section_id>") 拉取具体分节。
禁止一次性拉取全部归档正文；禁止把整段历史原文回填进提示词。
拉取后只提炼与当前问题直接相关的事实、约束和时间点，再继续回答。
