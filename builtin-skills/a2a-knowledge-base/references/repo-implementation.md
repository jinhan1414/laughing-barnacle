# 本仓库 A2A 实现

## 维护链路

- 维护接入走请求式链路：`/api/a2a/agents/save|toggle|delete`。
- 变更后应通过 `/api/a2a/agents*` 回读校验。
- 设置页与 JSON 接口都应反映当前已接入 A2A Agent 列表和状态。

## 执行链路

- 当前产品通过内置 A2A 能力执行，不依赖 Skill 直接拼 HTTP。
- 后端调用集中在 `internal/a2a/provider_sdk.go`。
- 参考服务在 `integrations/codex-a2a/`，用于本地联调和协议对照。

## 当前明确约束

- Agent Card 缺失 `skills` 或可执行 Agent 的 `skills` 为空，会被显式拒绝。
- 长耗时 A2A 任务采用“进行中/错误直返，禁止同轮轮询扩张”。
- 回答执行结果时，以工具回读与任务证据为准，不允许模型自行补完“已完成”。

## 解释项目问题时的默认结构

1. 标准上 A2A 是什么。
2. 本仓库当前在哪些位置实现。
3. 这次问题发生在维护链路还是执行链路。
4. 证据在哪里。
