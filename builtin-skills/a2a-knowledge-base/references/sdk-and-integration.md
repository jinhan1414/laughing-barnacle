# A2A SDK 与接入

## 优先回答路径

- 用户问 Go 接入：优先看 `a2a-go` 官方 SDK 和本仓库 `internal/a2a/provider_sdk.go`。
- 用户问 Python 接入：优先看 `a2a-python` 官方 SDK 和 `integrations/codex-a2a/`。
- 用户问“有没有 SDK”：直接先回答官方 SDK，再补本仓库怎么接。

## Go 口径

- 本仓库当前后端通过官方 `a2a-go` SDK 发起调用。
- 重点路径：Agent Card 发现、从 Card 创建 client、执行 `SendMessage` / `GetTask` / `CancelTask`。
- 若用户问“怎么接入”，优先讲发现 Card、校验 `skills`、创建 client、发请求、回读任务。

## Python 口径

- 本仓库参考服务 `integrations/codex-a2a` 使用官方 `a2a-python` SDK。
- 重点是 `/.well-known/agent-card.json` 与 `/a2a/rpc` 的暴露、任务执行器、任务状态托管。

## 实战回答模板

1. 发现远端 Agent：读取 Agent Card。
2. 校验可调用性：确认 `skills`、端点、鉴权。
3. 发起调用：send message 或同等请求。
4. 查询进度：get task。
5. 中断任务：cancel task。
6. 校验完成：看 task 状态、message、artifact、事件或日志。
